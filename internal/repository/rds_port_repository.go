package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/martin/network-service/internal/domain"
)

var (
	ErrNoAvailablePorts = errors.New("no available public ports in range 5432-6000")
)

type postgresRDSPortRepository struct {
	db *sqlx.DB
}

func NewRDSPortRepository(db *sqlx.DB) RDSPortRepository {
	return &postgresRDSPortRepository{db: db}
}

// Allocate finds the next free port in range 5432-6000 and inserts a new allocation
func (r *postgresRDSPortRepository) Allocate(ctx context.Context, tenantID, resourceID, privateIP string, privatePort int, publicIP string) (int, error) {
	var publicPort int

	// Transaction to safely find and reserve a port concurrently
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Check if resource already has an active allocation (idempotent setup)
	var existing domain.RDSPortAllocation
	err = tx.GetContext(ctx, &existing, `
		SELECT * FROM rds_port_allocations 
		WHERE resource_id = $1 AND released_at IS NULL
	`, resourceID)

	if err == nil {
		// Found existing active allocation
		return existing.PublicPort, nil
	} else if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed allocating querying existing ports: %w", err)
	}

	// 2. Find lowest available port in range 5432-6000
	err = tx.GetContext(ctx, &publicPort, `
		WITH available_ports AS (
			SELECT generate_series(5432, 6000) AS port
			EXCEPT
			SELECT public_port FROM rds_port_allocations WHERE released_at IS NULL
			ORDER BY port ASC LIMIT 1
		)
		SELECT port FROM available_ports
	`)

	if err == sql.ErrNoRows || publicPort == 0 {
		return 0, ErrNoAvailablePorts
	} else if err != nil {
		return 0, fmt.Errorf("failed allocating finding free port: %w", err)
	}

	// 3. Insert new allocation
	alloc := domain.RDSPortAllocation{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		ResourceID:  resourceID,
		PrivateIP:   privateIP,
		PrivatePort: privatePort,
		PublicIP:    publicIP,
		PublicPort:  publicPort,
	}

	query := `
		INSERT INTO rds_port_allocations 
		(id, tenant_id, resource_id, private_ip, private_port, public_ip, public_port, created_at)
		VALUES (:id, :tenant_id, :resource_id, :private_ip, :private_port, :public_ip, :public_port, NOW())
	`
	if _, err := tx.NamedExecContext(ctx, query, alloc); err != nil {
		return 0, fmt.Errorf("failed allocating inserting new port allocation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return publicPort, nil
}

// Release marks an allocation as released (freeing up the port)
func (r *postgresRDSPortRepository) Release(ctx context.Context, resourceID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE rds_port_allocations 
		SET released_at = NOW() 
		WHERE resource_id = $1 AND released_at IS NULL
	`, resourceID)
	return err
}

// GetByResourceID retrieves the active allocation for a resource
func (r *postgresRDSPortRepository) GetByResourceID(ctx context.Context, resourceID string) (*domain.RDSPortAllocation, error) {
	var alloc domain.RDSPortAllocation
	err := r.db.GetContext(ctx, &alloc, `
		SELECT * FROM rds_port_allocations 
		WHERE resource_id = $1 AND released_at IS NULL
	`, resourceID)
	if err == sql.ErrNoRows {
		return nil, nil // Return nil if not found instead of error to cleanly handle optional state
	}
	if err != nil {
		return nil, err
	}
	return &alloc, nil
}

// ListActive retrieves all current active allocations (useful for reconciling iptables on startup)
func (r *postgresRDSPortRepository) ListActive(ctx context.Context) ([]*domain.RDSPortAllocation, error) {
	var allocations []*domain.RDSPortAllocation
	err := r.db.SelectContext(ctx, &allocations, `
		SELECT * FROM rds_port_allocations 
		WHERE released_at IS NULL
	`)
	return allocations, err
}
