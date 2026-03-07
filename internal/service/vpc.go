package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/pkg/logger"
	"go.uber.org/zap"
)

func (s *networkService) CreateDefaultVPC(ctx context.Context, tenantID, tenantName string) (*domain.VPC, error) {
	l := logger.WithContext(ctx)
	l.Info("Starting default VPC creation", zap.String("tenant_id", tenantID))

	// 1. Idempotency Check
	existingDefaultVPC, err := s.vpcRepo.GetDefaultVPC(ctx, tenantID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check for existing default VPC: %w", err)
	}

	if existingDefaultVPC != nil {
		l.Info("Default VPC already exists for tenant, skipping creation", zap.String("tenant_id", tenantID), zap.String("vpc_id", existingDefaultVPC.ID))
		return existingDefaultVPC, nil
	}

	return s.provisionVPCInternal(ctx, tenantID, fmt.Sprintf("%s-vpc", tenantName), true)
}

func (s *networkService) CreateVPC(ctx context.Context, tenantID, vpcName string) (*domain.VPC, error) {
	l := logger.WithContext(ctx)
	l.Info("Starting custom VPC creation", zap.String("tenant_id", tenantID), zap.String("vpc_name", vpcName))

	// 1. Duplicate Name Check
	vpcs, err := s.vpcRepo.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list VPCs for duplicate check: %w", err)
	}

	for _, v := range vpcs {
		if v.Name == vpcName {
			l.Info("VPC with this name already exists for tenant", zap.String("tenant_id", tenantID), zap.String("vpc_name", vpcName))
			return &v, nil
		}
	}

	return s.provisionVPCInternal(ctx, tenantID, vpcName, false)
}

func (s *networkService) ListVPCs(ctx context.Context, tenantID string) ([]domain.VPC, error) {
	l := logger.WithContext(ctx)
	l.Info("Listing VPCs", zap.String("tenant_id", tenantID))

	vpcs, err := s.vpcRepo.ListByTenant(ctx, tenantID)
	if err != nil {
		l.Error("Failed to list VPCs", zap.Error(err), zap.String("tenant_id", tenantID))
		return nil, fmt.Errorf("failed to list VPCs: %w", err)
	}

	return vpcs, nil
}

func (s *networkService) provisionVPCInternal(ctx context.Context, tenantID, vpcName string, isDefault bool) (*domain.VPC, error) {
	l := logger.WithContext(ctx)

	// 1. Begin Transaction
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure rollback on panic or error if not committed
	defer func() {
		if tx != nil {
			if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
				l.Error("Failed to rollback transaction", zap.Error(err))
			}
		}
	}()

	// 2. Allocate CIDR (Always new for each VPC)
	vpcCIDR, err := s.cidrRepo.AllocateNextWithTx(ctx, tx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate CIDR: %w", err)
	}
	l.Info("Allocated CIDR for VPC", zap.String("tenant_id", tenantID), zap.String("cidr", vpcCIDR))

	octets := strings.Split(vpcCIDR, ".")
	if len(octets) < 2 {
		return nil, fmt.Errorf("invalid VPC CIDR: %s", vpcCIDR)
	}
	basePrefix := fmt.Sprintf("%s.%s", octets[0], octets[1])

	// 3. Generate Bridge Name (unique per VPC)
	vpcID := uuid.New().String()
	bridgeName := fmt.Sprintf("br-vpc-%s", vpcID[:8])

	// 4. Create VPC Record (Inside Transaction)
	vpc := &domain.VPC{
		ID:         vpcID,
		Name:       vpcName,
		CIDRBlock:  vpcCIDR,
		BridgeName: bridgeName,
		TenantID:   tenantID,
		Status:     domain.VPCStatusProvisioning,
		IsDefault:  isDefault,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	l.Info("Creating VPC record", zap.String("vpc_id", vpcID), zap.String("is_default", fmt.Sprint(isDefault)))
	if err := s.vpcRepo.CreateWithTx(ctx, tx, vpc); err != nil {
		return nil, fmt.Errorf("failed to create VPC: %w", err)
	}

	// 6. Create Route Table (Inside Transaction)
	rtID := uuid.New().String()
	rt := &domain.RouteTable{
		ID:        rtID,
		VPCID:     vpcID,
		Name:      "main-rt",
		CreatedAt: time.Now(),
	}
	l.Info("Creating Main Route Table", zap.String("rt_id", rtID))
	if err := s.rtRepo.CreateWithTx(ctx, tx, rt); err != nil {
		return nil, fmt.Errorf("failed to create route table: %w", err)
	}

	// 7. Create Public Subnet (Inside Transaction)
	publicSubnetID := uuid.New().String()
	publicSubnet := &domain.Subnet{
		ID:           publicSubnetID,
		VPCID:        vpcID,
		RouteTableID: rtID,
		Name:         "public-subnet",
		CIDRBlock:    fmt.Sprintf("%s.1.0/24", basePrefix),
		Az:           "us-east-1a",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	l.Info("Creating Public Subnet", zap.String("subnet_id", publicSubnetID), zap.String("cidr", publicSubnet.CIDRBlock))
	if err := s.subnetRepo.CreateWithTx(ctx, tx, publicSubnet); err != nil {
		return nil, fmt.Errorf("failed to create public subnet: %w", err)
	}

	// 8. Create Private Subnet (Inside Transaction)
	privateSubnetID := uuid.New().String()
	privateSubnet := &domain.Subnet{
		ID:           privateSubnetID,
		VPCID:        vpcID,
		RouteTableID: rtID,
		Name:         "private-subnet",
		CIDRBlock:    fmt.Sprintf("%s.2.0/24", basePrefix),
		Az:           "us-east-1a",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	l.Info("Creating Private Subnet", zap.String("subnet_id", privateSubnetID), zap.String("cidr", privateSubnet.CIDRBlock))
	if err := s.subnetRepo.CreateWithTx(ctx, tx, privateSubnet); err != nil {
		return nil, fmt.Errorf("failed to create private subnet: %w", err)
	}

	// 9. Create Internet Gateway (Inside Transaction)
	igwID := uuid.New().String()
	igw := &domain.InternetGateway{
		ID:        igwID,
		VPCID:     vpcID,
		Name:      "main-igw",
		CreatedAt: time.Now(),
	}
	l.Info("Creating Internet Gateway", zap.String("igw_id", igwID))
	if err := s.igwRepo.CreateWithTx(ctx, tx, igw); err != nil {
		return nil, fmt.Errorf("failed to create internet gateway: %w", err)
	}

	// 10. Add default route to IGW (Inside Transaction)
	route := &domain.Route{
		ID:              uuid.New().String(),
		RouteTableID:    rtID,
		DestinationCIDR: "0.0.0.0/0",
		TargetType:      "igw",
		TargetID:        igwID,
		CreatedAt:       time.Now(),
	}
	if err := s.routeRepo.CreateWithTx(ctx, tx, route); err != nil {
		return nil, fmt.Errorf("failed to create default route: %w", err)
	}

	// 11. Create Default Security Group (Inside Transaction)
	sgID := uuid.New().String()
	sg := &domain.SecurityGroup{
		ID:          sgID,
		VPCID:       vpcID,
		Name:        "default-sg",
		Description: "Default security group allowing all internal traffic",
		CreatedAt:   time.Now(),
	}
	l.Info("Creating Default Security Group", zap.String("sg_id", sgID))
	if err := s.sgRepo.CreateWithTx(ctx, tx, sg); err != nil {
		return nil, fmt.Errorf("failed to create security group: %w", err)
	}

	// 12. Commit Transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	tx = nil // Prevent rollback on defer
	l.Info("Control plane transaction committed for VPC", zap.String("vpc_id", vpcID))

	// 13. OS Provisioning (Outside Transaction)
	l.Info("Starting OS provisioning for VPC", zap.String("vpc_id", vpcID), zap.String("bridge", bridgeName))

	// 13.1 Create Physical Bridge
	if err := s.bridgeDriver.CreateBridge(bridgeName); err != nil {
		l.Error("OS provisioning failed: bridge creation error", zap.Error(err), zap.String("vpc_id", vpcID))
		s.vpcRepo.UpdateStatus(ctx, vpcID, domain.VPCStatusError)
		return nil, fmt.Errorf("failed to create bridge: %w", err)
	}

	// 13.2 Register bridge with Docker so containers can attach to it
	// We use the public subnet CIDR for this. The gateway is the first IP in that subnet.
	gatewayIP := DeriveGateway(publicSubnet.CIDRBlock)
	if gatewayIP != "" {
		if err := s.dockerNetworkDriver.RegisterBridge(bridgeName, publicSubnet.CIDRBlock, gatewayIP); err != nil {
			l.Warn("Failed to register bridge with Docker", zap.Error(err), zap.String("bridge", bridgeName))
			// non-fatal for EC2 VMs, but might cause issues for RDS containers later
		}
	} else {
		l.Error("Failed to derive gateway IP for Docker network registration", zap.String("cidr", publicSubnet.CIDRBlock))
	}

	// ALWAYS ensure the bridge is UP.
	if err := s.bridgeDriver.BringUp(bridgeName); err != nil {
		l.Warn("Failed to bring up bridge during provisioning", zap.Error(err), zap.String("bridge", bridgeName))
	}

	// Assign gateway IPs for ALL subnets in this VPC.
	// Each subnet x.x.y.0/24 gets a gateway at x.x.y.1 on the bridge.
	subnets, err := s.subnetRepo.ListByVPC(ctx, vpcID)
	if err != nil {
		l.Error("OS provisioning failed: could not list subnets for IP assignment", zap.Error(err), zap.String("vpc_id", vpcID))
	} else {
		for _, sn := range subnets {
			gateway := DeriveGateway(sn.CIDRBlock)
			if gateway != "" {
				gatewayCIDR := fmt.Sprintf("%s/24", gateway)
				if err := s.bridgeDriver.AssignIP(bridgeName, gatewayCIDR); err != nil {
					l.Error("OS provisioning: gateway IP assignment error", zap.Error(err), zap.String("subnet", sn.ID), zap.String("ip", gatewayCIDR))
				} else {
					l.Info("Gateway IP assigned to bridge", zap.String("subnet", sn.ID), zap.String("cidr", gatewayCIDR))
				}
			}
		}
	}

	// 13.2 Setup MASQUERADE
	if err := s.iptablesDriver.SetupMasquerade(publicSubnet.CIDRBlock); err != nil {
		l.Error("OS provisioning failed: MASQUERADE setup error", zap.Error(err), zap.String("vpc_id", vpcID))
		s.vpcRepo.UpdateStatus(ctx, vpcID, domain.VPCStatusError)
		return nil, fmt.Errorf("failed to setup MASQUERADE: %w", err)
	}

	// 14. Update status to active
	if err := s.vpcRepo.UpdateStatus(ctx, vpcID, domain.VPCStatusActive); err != nil {
		l.Error("Failed to update VPC status to active", zap.Error(err), zap.String("vpc_id", vpcID))
		return nil, fmt.Errorf("failed to update VPC status: %w", err)
	}

	l.Info("VPC provisioning completed successfully", zap.String("tenant_id", tenantID), zap.String("vpc_id", vpcID))
	return vpc, nil
}

func (s *networkService) GetDefaultVPC(ctx context.Context, tenantID string) (*domain.VPC, error) {
	l := logger.WithContext(ctx).With(zap.String("tenant_id", tenantID))
	vpc, err := s.vpcRepo.GetDefaultVPC(ctx, tenantID)
	if err != nil {
		if err == sql.ErrNoRows {
			l.Info("No default VPC found, creating one on-the-fly")
			// Use tenantID as the name for now as a fallback
			return s.CreateDefaultVPC(ctx, tenantID, tenantID)
		}
		return nil, err
	}
	return vpc, nil
}

func (s *networkService) ValidateVPC(ctx context.Context, tenantID, vpcID string) (bool, string, string, error) {
	l := logger.WithContext(ctx)
	l.Info("Validating VPC ownership and status", zap.String("tenant_id", tenantID), zap.String("vpc_id", vpcID))

	vpc, err := s.vpcRepo.GetByID(ctx, vpcID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, "", "VPC not found", nil
		}
		return false, "", "", fmt.Errorf("failed to get VPC: %w", err)
	}

	if vpc.TenantID != tenantID {
		l.Warn("VPC ownership mismatch", zap.String("expected_tenant", tenantID), zap.String("actual_tenant", vpc.TenantID))
		return false, vpc.Status, "VPC does not belong to tenant", nil
	}

	if vpc.Status != domain.VPCStatusActive {
		return false, vpc.Status, fmt.Sprintf("VPC is in %s status (likely due to missing bridge or permissions)", vpc.Status), nil
	}

	return true, vpc.Status, "", nil
}

func (s *networkService) ResolveDefaultNetwork(ctx context.Context, tenantID string) (string, string, string, error) {
	l := logger.WithContext(ctx).With(zap.String("tenant_id", tenantID))
	l.Info("Received default network resolution request")

	// 1. Find default VPC
	vpc, err := s.vpcRepo.GetDefaultVPC(ctx, tenantID)
	if err != nil {
		if err == sql.ErrNoRows {
			l.Warn("No default VPC found")
			return "", "", "", fmt.Errorf("no default VPC found for tenant %s", tenantID)
		}
		l.Error("Failed to get default VPC", zap.Error(err))
		return "", "", "", fmt.Errorf("failed to get default VPC: %w", err)
	}

	return s.ResolveVPCNetwork(ctx, tenantID, vpc.ID)
}

func (s *networkService) ResolveVPCNetwork(ctx context.Context, tenantID, vpcID string) (string, string, string, error) {
	l := logger.WithContext(ctx).With(zap.String("tenant_id", tenantID), zap.String("vpc_id", vpcID))
	l.Info("Resolving network for VPC")

	// 1. Get VPC
	vpc, err := s.vpcRepo.GetByID(ctx, vpcID)
	if err != nil {
		if err == sql.ErrNoRows {
			l.Warn("VPC not found", zap.String("vpc_id", vpcID))
			return "", "", "", fmt.Errorf("VPC not found: %s", vpcID)
		}
		l.Error("Failed to get VPC", zap.Error(err))
		return "", "", "", fmt.Errorf("failed to get VPC: %w", err)
	}

	if vpc.TenantID != tenantID {
		l.Warn("VPC ownership mismatch", zap.String("expected_tenant", tenantID), zap.String("actual_tenant", vpc.TenantID))
		return "", "", "", fmt.Errorf("VPC %s does not belong to tenant %s", vpcID, tenantID)
	}

	// 2. Find public subnet in that VPC
	subnets, err := s.subnetRepo.ListByVPC(ctx, vpc.ID)
	if err != nil {
		l.Error("Failed to list subnets for VPC", zap.Error(err), zap.String("vpc_id", vpc.ID))
		return "", "", "", fmt.Errorf("failed to list subnets for VPC: %w", err)
	}

	var defaultSubnetID string
	for _, sn := range subnets {
		// Public subnet is typically named with "public"
		if strings.Contains(strings.ToLower(sn.Name), "public") {
			defaultSubnetID = sn.ID
			break
		}
	}

	// Fallback to first subnet if no public one found
	if defaultSubnetID == "" && len(subnets) > 0 {
		defaultSubnetID = subnets[0].ID
		l.Warn("No explicit public subnet found, falling back to first available",
			zap.String("vpc_id", vpc.ID),
			zap.String("fallback_subnet", defaultSubnetID))
	}

	if defaultSubnetID == "" {
		l.Warn("No subnets found in VPC", zap.String("vpc_id", vpc.ID))
		return "", "", "", fmt.Errorf("no subnets found in VPC %s", vpc.ID)
	}

	l.Info("Resolved network for VPC",
		zap.String("vpc_id", vpc.ID),
		zap.String("subnet_id", defaultSubnetID))

	return vpc.ID, defaultSubnetID, vpc.BridgeName, nil
}
