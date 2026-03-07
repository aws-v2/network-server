package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/pkg/logger"
	"go.uber.org/zap"
)

func (s *networkService) AutoAssociateEIP(ctx context.Context, instanceID, privateIP string) (*domain.ElasticIP, error) {
	l := logger.WithContext(ctx).With(zap.String("instance_id", instanceID), zap.String("private_ip", privateIP))
	l.Info("Auto-associating EIP for instance")

	// 1. Allocate
	eip, err := s.AllocateEIP(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate EIP for auto-association: %w", err)
	}

	// 2. Associate
	if err := s.AssociateEIP(ctx, eip.ID, instanceID, privateIP); err != nil {
		return nil, fmt.Errorf("failed to associate EIP for auto-association: %w", err)
	}

	return eip, nil
}

func (s *networkService) AllocateEIP(ctx context.Context) (*domain.ElasticIP, error) {
	eipID := uuid.New().String()
	// In a real system, we'd pull from a pool. For now, we'll generate a random public IP for simulation.
	// Let's use 203.0.113.x (TEST-NET-3 range)
	publicIP := fmt.Sprintf("203.0.113.%d", time.Now().UnixNano()%254+1)

	eip := &domain.ElasticIP{
		ID:        eipID,
		PublicIP:  publicIP,
		Allocated: false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.eipRepo.Create(ctx, eip); err != nil {
		return nil, fmt.Errorf("failed to create EIP record: %w", err)
	}

	return eip, nil
}

func (s *networkService) AssociateEIP(ctx context.Context, eipID, instanceID, privateIP string) error {
	l := logger.WithContext(ctx)

	eip, err := s.eipRepo.GetByID(ctx, eipID)
	if err != nil {
		return fmt.Errorf("failed to find EIP: %w", err)
	}

	if eip.Allocated && eip.InstanceID != "" {
		// Disassociate first
		if err := s.DisassociateEIP(ctx, eipID); err != nil {
			return fmt.Errorf("failed to disassociate existing assignment: %w", err)
		}
	}

	// 1. Update record in transaction
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	eip.Allocated = true
	eip.InstanceID = instanceID
	eip.PrivateIP = privateIP
	eip.UpdatedAt = time.Now()

	if err := s.eipRepo.UpdateWithTx(ctx, tx, eip); err != nil {
		return fmt.Errorf("failed to update EIP record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 2. Setup host IP alias (so host responds to ARP for this IP)
	if err := s.bridgeDriver.AddIPAlias(eip.PublicIP, s.publicInterface); err != nil {
		l.Error("Failed to add host IP alias", zap.Error(err), zap.String("public_ip", eip.PublicIP))
		// We proceed, as iptables might still work if the IP is routed to us,
		// but usually aliasing is needed for local reachability.
	}

	// 3. Setup iptables rules
	if err := s.iptablesDriver.SetupDNAT(eip.PublicIP, privateIP); err != nil {
		l.Error("Failed to setup DNAT", zap.Error(err), zap.String("public_ip", eip.PublicIP))
	}
	if err := s.iptablesDriver.SetupSNAT(privateIP, eip.PublicIP); err != nil {
		l.Error("Failed to setup SNAT", zap.Error(err), zap.String("public_ip", eip.PublicIP))
	}

	l.Info("Associated EIP", zap.String("eip", eip.PublicIP), zap.String("instance_id", instanceID), zap.String("private_ip", privateIP))
	return nil
}

func (s *networkService) DisassociateEIP(ctx context.Context, eipID string) error {
	l := logger.WithContext(ctx)

	eip, err := s.eipRepo.GetByID(ctx, eipID)
	if err != nil {
		return fmt.Errorf("failed to find EIP: %w", err)
	}

	if !eip.Allocated || (eip.InstanceID == "" && eip.PrivateIP == "") {
		return nil
	}

	// Capture values for iptables removal before clearing them in DB
	oldPrivateIP := eip.PrivateIP

	// 1. Update record in transaction
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	eip.Allocated = false
	eip.InstanceID = ""
	eip.PrivateIP = ""
	eip.UpdatedAt = time.Now()

	if err := s.eipRepo.UpdateWithTx(ctx, tx, eip); err != nil {
		return fmt.Errorf("failed to update EIP record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 2. Cleanup host and network rules
	if err := s.bridgeDriver.RemoveIPAlias(eip.PublicIP, s.publicInterface); err != nil {
		l.Error("Failed to remove host IP alias", zap.Error(err), zap.String("public_ip", eip.PublicIP))
	}

	if oldPrivateIP != "" {
		if err := s.iptablesDriver.RemoveDNAT(eip.PublicIP, oldPrivateIP); err != nil {
			l.Error("Failed to remove DNAT", zap.Error(err), zap.String("public_ip", eip.PublicIP))
		}
		if err := s.iptablesDriver.RemoveSNAT(oldPrivateIP, eip.PublicIP); err != nil {
			l.Error("Failed to remove SNAT", zap.Error(err), zap.String("public_ip", eip.PublicIP))
		}
	} else {
		l.Warn("Disassociating EIP: Iptables rules might remain because private_ip was unknown", zap.String("eip", eip.PublicIP))
	}

	l.Info("Disassociated EIP", zap.String("eip", eip.PublicIP))
	return nil
}
