package service

import (
	"context"
	"fmt"
	"time"

	"strings"

	"database/sql"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/internal/driver"
	"github.com/martin/network-service/internal/registry"
	"github.com/martin/network-service/internal/repository"
	"github.com/martin/network-service/pkg/logger"
	"go.uber.org/zap"
)

type NetworkService interface {
	CreateDefaultVPC(ctx context.Context, tenantID, tenantName string) (*domain.VPC, error)
	CreateVPC(ctx context.Context, tenantID, vpcName string) (*domain.VPC, error)

	// Elastic IP Management
	AllocateEIP(ctx context.Context) (*domain.ElasticIP, error)
	AssociateEIP(ctx context.Context, eipID, instanceID, privateIP string) error
	DisassociateEIP(ctx context.Context, eipID string) error

	// VPC Resolution & Validation
	GetDefaultVPC(ctx context.Context, tenantID string) (*domain.VPC, error)
	ValidateVPC(ctx context.Context, tenantID, vpcID string) (bool, string, string, error)
	ResolveDefaultNetwork(ctx context.Context, tenantID string) (string, string, string, error)

	// Resource Assignments (Legacy VPC-only - deprecated)
	AttachResourceToVPC(ctx context.Context, tenantID, resourceARN, vpcID string) error
	DetachResourceFromVPC(ctx context.Context, resourceARN string) error

	// Resource Networking (Subnet-level)
	AttachResourceToSubnet(ctx context.Context, tenantID, resourceARN, vpcID, subnetID string) error
	DetachResourceFromNetwork(ctx context.Context, resourceARN string) error
	ListResourcesInVPC(ctx context.Context, tenantID, vpcID string) ([]domain.ResourceNetworkAssignment, error)
	ResolveResourceNetwork(ctx context.Context, resourceARN string) (*domain.ResourceNetworkAssignment, error)

	// Reconciliation
	ReconcileVPCs(ctx context.Context) error

	// Compute Registry (Phase 12)
	RegisterComputeInstance(ctx context.Context, instance domain.ComputeInstance) error
	UpdateComputeInstanceHealth(ctx context.Context, instanceID string, status domain.InstanceStatus) error
	DeregisterComputeInstance(ctx context.Context, instanceID string) error
	GetActiveComputeInstances(ctx context.Context) ([]domain.ComputeInstance, error)
	HandleComputeLifecycleEvent(ctx context.Context, event domain.ComputeLifecycleEvent) error
	SelectComputeRoute(ctx context.Context) (*domain.ComputeInstance, error)
	MonitorComputeHealth(ctx context.Context) error
}

type networkService struct {
	db             *sqlx.DB
	vpcRepo        repository.VPCRepository
	subnetRepo     repository.SubnetRepository
	igwRepo        repository.InternetGatewayRepository
	rtRepo         repository.RouteTableRepository
	routeRepo      repository.RouteRepository
	sgRepo         repository.SecurityGroupRepository
	cidrRepo       repository.CIDRRepository
	eipRepo        repository.ElasticIPRepository
	resourceRepo   repository.ResourceVPCRepository
	netAssignRepo  repository.ResourceNetworkRepository
	computeReg     registry.ComputeRegistry
	bridgeDriver   driver.BridgeDriver
	iptablesDriver driver.IptablesDriver
	routingDriver  driver.RoutingDriver
}

func NewNetworkService(
	db *sqlx.DB,
	vpcRepo repository.VPCRepository,
	subnetRepo repository.SubnetRepository,
	igwRepo repository.InternetGatewayRepository,
	rtRepo repository.RouteTableRepository,
	routeRepo repository.RouteRepository,
	sgRepo repository.SecurityGroupRepository,
	cidrRepo repository.CIDRRepository,
	eipRepo repository.ElasticIPRepository,
	bridgeDriver driver.BridgeDriver,
	iptablesDriver driver.IptablesDriver,
	routingDriver driver.RoutingDriver,
	resourceRepo repository.ResourceVPCRepository,
	netAssignRepo repository.ResourceNetworkRepository,
	computeReg registry.ComputeRegistry,
) NetworkService {
	return &networkService{
		db:             db,
		vpcRepo:        vpcRepo,
		subnetRepo:     subnetRepo,
		igwRepo:        igwRepo,
		rtRepo:         rtRepo,
		routeRepo:      routeRepo,
		sgRepo:         sgRepo,
		cidrRepo:       cidrRepo,
		eipRepo:        eipRepo,
		resourceRepo:   resourceRepo,
		netAssignRepo:  netAssignRepo,
		computeReg:     computeReg,
		bridgeDriver:   bridgeDriver,
		iptablesDriver: iptablesDriver,
		routingDriver:  routingDriver,
	}
}

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

	gatewayCIDR := fmt.Sprintf("%s.1.1/24", basePrefix)
	if err := s.bridgeDriver.AssignIP(bridgeName, gatewayCIDR); err != nil {
		l.Error("OS provisioning failed: bridge IP assignment error", zap.Error(err), zap.String("vpc_id", vpcID))
		s.vpcRepo.UpdateStatus(ctx, vpcID, domain.VPCStatusError)
		return nil, fmt.Errorf("failed to assign IP to bridge: %w", err)
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
		return false, vpc.Status, fmt.Sprintf("VPC is in %s status", vpc.Status), nil
	}

	return true, vpc.Status, "", nil
}

func (s *networkService) AttachResourceToVPC(ctx context.Context, tenantID, resourceARN, vpcID string) error {
	l := logger.WithContext(ctx)
	l.Info("Attaching resource to VPC", zap.String("resource_arn", resourceARN), zap.String("vpc_id", vpcID))

	// 1. Validate VPC
	valid, status, reason, err := s.ValidateVPC(ctx, tenantID, vpcID)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("VPC validation failed (status: %s): %s", status, reason)
	}

	// 2. Check current assignment (Idempotency)
	currentVPC, err := s.resourceRepo.GetByResource(ctx, resourceARN)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check current resource assignment: %w", err)
	}

	if currentVPC == vpcID {
		l.Info("Resource already attached to target VPC", zap.String("resource_arn", resourceARN), zap.String("vpc_id", vpcID))
		return nil
	}

	if currentVPC != "" {
		l.Info("Moving resource between VPCs", zap.String("resource_arn", resourceARN), zap.String("old_vpc", currentVPC), zap.String("new_vpc", vpcID))
	}

	// 3. Persist assignment
	if err := s.resourceRepo.Assign(ctx, tenantID, resourceARN, vpcID); err != nil {
		return fmt.Errorf("failed to assign resource to VPC: %w", err)
	}

	l.Info("Successfully attached resource to VPC", zap.String("resource_arn", resourceARN), zap.String("vpc_id", vpcID))
	return nil
}

func (s *networkService) DetachResourceFromVPC(ctx context.Context, resourceARN string) error {
	l := logger.WithContext(ctx)
	l.Info("Detaching resource from VPC", zap.String("resource_arn", resourceARN))

	if err := s.resourceRepo.Detach(ctx, resourceARN); err != nil {
		return fmt.Errorf("failed to detach resource: %w", err)
	}

	return nil
}

func (s *networkService) AttachResourceToSubnet(ctx context.Context, tenantID, resourceARN, vpcID, subnetID string) error {
	l := logger.WithContext(ctx).With(
		zap.String("tenant_id", tenantID),
		zap.String("resource_arn", resourceARN),
		zap.String("vpc_id", vpcID),
		zap.String("subnet_id", subnetID),
	)
	l.Info("Received attach resource request")

	// 1. Validate VPC exists and belongs to tenant
	valid, status, reason, err := s.ValidateVPC(ctx, tenantID, vpcID)
	if err != nil {
		l.Error("VPC validation failed with error", zap.Error(err))
		return err
	}
	if !valid {
		l.Warn("VPC validation failed", zap.String("status", status), zap.String("reason", reason))
		return fmt.Errorf("VPC validation failed (status: %s): %s", status, reason)
	}

	// 2. Validate subnet belongs to VPC
	subnet, err := s.subnetRepo.GetByID(ctx, subnetID)
	if err != nil {
		if err == sql.ErrNoRows {
			l.Warn("Subnet not found")
			return fmt.Errorf("subnet not found: %s", subnetID)
		}
		l.Error("Failed to fetch subnet", zap.Error(err))
		return fmt.Errorf("failed to get subnet: %w", err)
	}
	if subnet.VPCID != vpcID {
		l.Warn("Subnet VPC mismatch", zap.String("subnet_vpc_id", subnet.VPCID))
		return fmt.Errorf("subnet %s does not belong to VPC %s", subnetID, vpcID)
	}

	// 3. Allocate private IP (temporary placeholder removed for now)
	privateIP := ""

	// 4. Store assignment via repository (Idempotent UPSERT)
	if err := s.netAssignRepo.AssignResource(ctx, tenantID, resourceARN, vpcID, subnetID, privateIP); err != nil {
		l.Error("Failed to persist network assignment", zap.Error(err))
		return fmt.Errorf("failed to store network assignment: %w", err)
	}

	l.Info("Attached resource to subnet")

	return nil
}

func (s *networkService) DetachResourceFromNetwork(ctx context.Context, resourceARN string) error {
	l := logger.WithContext(ctx).With(zap.String("resource_arn", resourceARN))
	l.Info("Received detach resource request")

	// 1. Find assignment (check if it exists)
	assignment, err := s.netAssignRepo.GetByResource(ctx, resourceARN)
	if err != nil {
		if err == sql.ErrNoRows {
			l.Info("Resource already detached or never attached (idempotent success)")
			return nil
		}
		l.Error("Failed to check current assignment", zap.Error(err))
		return fmt.Errorf("failed to check current assignment: %w", err)
	}

	// Enrich logging with VPC/Subnet info if found
	l = l.With(zap.String("vpc_id", assignment.VPCID), zap.String("subnet_id", assignment.SubnetID))

	// 2. Delete mapping
	if err := s.netAssignRepo.DetachResource(ctx, resourceARN); err != nil {
		l.Error("Failed to delete network mapping", zap.Error(err))
		return fmt.Errorf("failed to detach resource from network: %w", err)
	}

	l.Info("Detached resource from network")
	return nil
}

func (s *networkService) ListResourcesInVPC(ctx context.Context, tenantID, vpcID string) ([]domain.ResourceNetworkAssignment, error) {
	l := logger.WithContext(ctx).With(zap.String("tenant_id", tenantID), zap.String("vpc_id", vpcID))
	l.Info("Listing resources in VPC")

	// 1. Validate tenant ownership
	valid, _, reason, err := s.ValidateVPC(ctx, tenantID, vpcID)
	if err != nil {
		l.Error("VPC verification failed with error", zap.Error(err))
		return nil, err
	}
	if !valid {
		l.Warn("VPC verification failed", zap.String("reason", reason))
		return nil, fmt.Errorf("VPC verification failed: %s", reason)
	}

	// 2. Query repository
	assignments, err := s.netAssignRepo.ListResourcesInVPC(ctx, vpcID)
	if err != nil {
		l.Error("Failed to list resources in VPC", zap.Error(err))
		return nil, fmt.Errorf("failed to list resources in VPC: %w", err)
	}

	l.Info("Successfully listed resources", zap.Int("count", len(assignments)))
	return assignments, nil
}

func (s *networkService) ResolveResourceNetwork(ctx context.Context, resourceARN string) (*domain.ResourceNetworkAssignment, error) {
	l := logger.WithContext(ctx).With(zap.String("resource_arn", resourceARN))
	l.Info("Received resource network resolution request")

	assignment, err := s.netAssignRepo.GetByResource(ctx, resourceARN)
	if err != nil {
		if err == sql.ErrNoRows {
			l.Warn("Network assignment not found")
			return nil, fmt.Errorf("network assignment not found for resource: %s", resourceARN)
		}
		l.Error("Failed to resolve resource network", zap.Error(err))
		return nil, fmt.Errorf("failed to resolve resource network: %w", err)
	}

	l.Info("Resolved resource network location",
		zap.String("vpc_id", assignment.VPCID),
		zap.String("subnet_id", assignment.SubnetID),
		zap.String("private_ip", assignment.PrivateIP))

	return assignment, nil
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

	// 2. Find public subnet in that VPC
	subnets, err := s.subnetRepo.ListByVPC(ctx, vpc.ID)
	if err != nil {
		l.Error("Failed to list subnets for VPC", zap.Error(err), zap.String("vpc_id", vpc.ID))
		return "", "", "", fmt.Errorf("failed to list subnets for default VPC: %w", err)
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
		return "", "", "", fmt.Errorf("no subnets found in default VPC %s", vpc.ID)
	}

	l.Info("Resolved default network",
		zap.String("vpc_id", vpc.ID),
		zap.String("subnet_id", defaultSubnetID))

	return vpc.ID, defaultSubnetID, vpc.BridgeName, nil
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

	// 2. Setup iptables rules
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

	// 2. Remove iptables rules using the stored private_ip
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

func (s *networkService) CreateRouteTable(ctx context.Context, vpcID, name string) (*domain.RouteTable, error) {
	rtID := uuid.New().String()
	rt := &domain.RouteTable{
		ID:        rtID,
		VPCID:     vpcID,
		Name:      name,
		CreatedAt: time.Now(),
	}

	if err := s.rtRepo.Create(ctx, rt); err != nil {
		return nil, err
	}
	return rt, nil
}

func (s *networkService) AssociateSubnetToRouteTable(ctx context.Context, subnetID, rtID string) error {
	l := logger.WithContext(ctx)

	// 1. Update database
	if err := s.subnetRepo.AssociateRouteTable(ctx, subnetID, rtID); err != nil {
		return err
	}

	// 2. Fetch details for OS orchestration
	subnet, err := s.subnetRepo.GetByID(ctx, subnetID)
	if err != nil {
		return fmt.Errorf("failed to fetch subnet: %w", err)
	}

	vpc, err := s.vpcRepo.GetByID(ctx, subnet.VPCID)
	if err != nil {
		return fmt.Errorf("failed to fetch VPC: %w", err)
	}

	rt, err := s.rtRepo.GetFullByID(ctx, rtID)
	if err != nil {
		return fmt.Errorf("failed to fetch route table details: %w", err)
	}

	// 3. Apply existing routes to the bridge
	for _, route := range rt.Routes {
		// For now, we only apply routes that have a target ID (which we treat as the gateway for now)
		// and skip those that don't make sense for a direct 'ip route add ... via ... dev ...'
		if route.TargetID != "" {
			if err := s.routingDriver.AddRoute(vpc.BridgeName, route.DestinationCIDR, route.TargetID); err != nil {
				l.Error("Failed to apply route during association",
					zap.Error(err),
					zap.String("bridge", vpc.BridgeName),
					zap.String("destination", route.DestinationCIDR))
			}
		}
	}

	l.Info("Associated subnet to route table and applied routes",
		zap.String("subnet_id", subnetID),
		zap.String("rt_id", rtID),
		zap.String("bridge", vpc.BridgeName))
	return nil
}

func (s *networkService) CreateRoute(ctx context.Context, rtID, destination, targetType, targetID string) error {
	l := logger.WithContext(ctx)

	// 1. Save in database
	route := &domain.Route{
		ID:              uuid.New().String(),
		RouteTableID:    rtID,
		DestinationCIDR: destination,
		TargetType:      targetType,
		TargetID:        targetID,
		CreatedAt:       time.Now(),
	}

	if err := s.routeRepo.Create(ctx, route); err != nil {
		return err
	}

	// 2. Orhcestrate at OS level for affected subnets
	subnets, err := s.subnetRepo.ListByRouteTable(ctx, rtID)
	if err != nil {
		l.Error("Failed to list associated subnets for routing", zap.Error(err), zap.String("rt_id", rtID))
		return nil // Non-blocking for DB success
	}

	for _, sn := range subnets {
		vpc, err := s.vpcRepo.GetByID(ctx, sn.VPCID)
		if err != nil {
			l.Error("Failed to fetch VPC for routing orchestration", zap.Error(err), zap.String("vpc_id", sn.VPCID))
			continue
		}

		// Apply route to the bridge
		// Treating TargetID as gateway for IP routes
		if targetID != "" {
			if err := s.routingDriver.AddRoute(vpc.BridgeName, destination, targetID); err != nil {
				l.Error("Failed to apply route to system",
					zap.Error(err),
					zap.String("bridge", vpc.BridgeName),
					zap.String("destination", destination))
			}
		}
	}

	l.Info("Successfully created route and applied to OS",
		zap.String("rt_id", rtID),
		zap.String("destination", destination),
		zap.String("target", targetID),
		zap.Int("affected_subnets", len(subnets)))
	return nil
}

func (s *networkService) ReconcileVPCs(ctx context.Context) error {
	l := logger.WithContext(ctx)
	l.Info("Starting VPC reconciliation loop")

	vpcs, err := s.vpcRepo.ListIncomplete(ctx)
	if err != nil {
		return fmt.Errorf("failed to list incomplete VPCs: %w", err)
	}

	if len(vpcs) == 0 {
		l.Info("No VPCs requiring reconciliation found")
		return nil
	}

	for _, vpc := range vpcs {
		l.Info("Reconciling VPC", zap.String("vpc_id", vpc.ID), zap.String("status", vpc.Status))

		// 1. Re-run bridge creation
		if err := s.bridgeDriver.CreateBridge(vpc.BridgeName); err != nil {
			l.Error("Reconciliation failed: bridge creation error", zap.Error(err), zap.String("vpc_id", vpc.ID))
			continue
		}

		octets := strings.Split(vpc.CIDRBlock, ".")
		if len(octets) >= 2 {
			basePrefix := fmt.Sprintf("%s.%s", octets[0], octets[1])
			gatewayCIDR := fmt.Sprintf("%s.1.1/24", basePrefix)
			if err := s.bridgeDriver.AssignIP(vpc.BridgeName, gatewayCIDR); err != nil {
				l.Error("Reconciliation failed: bridge IP assignment error", zap.Error(err), zap.String("vpc_id", vpc.ID))
			}
		}

		// 2. Re-run iptables setup for subnets
		subnets, err := s.subnetRepo.ListByVPC(ctx, vpc.ID)
		if err != nil {
			l.Error("Reconciliation failed: failed to list subnets", zap.Error(err), zap.String("vpc_id", vpc.ID))
			continue
		}

		for _, sn := range subnets {
			// In our current default VPC setup, "public-subnet" gets MASQUERADE
			if strings.Contains(sn.Name, "public") {
				if err := s.iptablesDriver.SetupMasquerade(sn.CIDRBlock); err != nil {
					l.Error("Reconciliation failed: MASQUERADE setup error", zap.Error(err), zap.String("vpc_id", vpc.ID), zap.String("subnet_id", sn.ID))
					// We continue to see if other subnets or status update can still work, or maybe it's just one rule
				}
			}
		}

		// 3. Mark as active
		if err := s.vpcRepo.UpdateStatus(ctx, vpc.ID, domain.VPCStatusActive); err != nil {
			l.Error("Reconciliation failed: status update error", zap.Error(err), zap.String("vpc_id", vpc.ID))
			continue
		}

		l.Info("VPC reconciled successfully", zap.String("vpc_id", vpc.ID))
	}

	return nil
}

// Phase 12: Compute Registry Methods

func (s *networkService) RegisterComputeInstance(ctx context.Context, instance domain.ComputeInstance) error {
	l := logger.WithContext(ctx).With(zap.String("instance_id", instance.InstanceID))
	l.Info("Registering compute instance", zap.String("ip", instance.IPAddress))
	return s.computeReg.Register(instance)
}

func (s *networkService) UpdateComputeInstanceHealth(ctx context.Context, instanceID string, status domain.InstanceStatus) error {
	l := logger.WithContext(ctx).With(zap.String("instance_id", instanceID))
	l.Info("Updating compute instance health", zap.String("status", string(status)))
	return s.computeReg.UpdateHealth(instanceID, status)
}

func (s *networkService) DeregisterComputeInstance(ctx context.Context, instanceID string) error {
	l := logger.WithContext(ctx).With(zap.String("instance_id", instanceID))
	l.Info("Deregistering compute instance")
	return s.computeReg.Deregister(instanceID)
}

func (s *networkService) GetActiveComputeInstances(ctx context.Context) ([]domain.ComputeInstance, error) {
	l := logger.WithContext(ctx)
	l.Debug("Fetching active compute instances")
	return s.computeReg.GetActiveInstances()
}

func (s *networkService) HandleComputeLifecycleEvent(ctx context.Context, event domain.ComputeLifecycleEvent) error {
	l := logger.WithContext(ctx).With(
		zap.String("instance_id", event.InstanceID),
		zap.String("event_type", string(event.EventType)),
	)
	l.Info("Processing compute lifecycle event")

	switch event.EventType {
	case domain.ComputeEventInstanceStarted:
		inst := domain.ComputeInstance{
			InstanceID:  event.InstanceID,
			IPAddress:   event.Payload.IPAddress,
			ServicePort: event.Payload.ServicePort,
			Status:      domain.InstanceStatusStarting,
			Metadata:    event.Payload.Metadata,
		}
		return s.computeReg.Register(inst)

	case domain.ComputeEventInstanceStopped:
		return s.computeReg.Deregister(event.InstanceID)

	case domain.ComputeEventHealthUpdate:
		err := s.computeReg.UpdateHealth(event.InstanceID, domain.InstanceStatusHealthy)
		if err != nil && strings.Contains(err.Error(), "instance not found") {
			l.Info("Instance not found for health update, registering automatically")
			inst := domain.ComputeInstance{
				InstanceID:  event.InstanceID,
				IPAddress:   event.Payload.IPAddress,
				ServicePort: event.Payload.ServicePort,
				Status:      domain.InstanceStatusHealthy,
				Metadata:    event.Payload.Metadata,
			}
			return s.computeReg.Register(inst)
		}
		return err

	default:
		l.Warn("Unsupported compute lifecycle event type")
		return nil
	}
}

func (s *networkService) SelectComputeRoute(ctx context.Context) (*domain.ComputeInstance, error) {
	l := logger.WithContext(ctx)
	l.Info("Selecting compute route")
	return s.computeReg.SelectRoute()
}

func (s *networkService) MonitorComputeHealth(ctx context.Context) error {
	zap.L().Info("Starting compute health monitor")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	threshold := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			zap.L().Info("Stopping compute health monitor")
			return ctx.Err()
		case <-ticker.C:
			evicted, err := s.computeReg.CleanupStaleInstances(threshold)
			if err != nil {
				zap.L().Error("Compute health cleanup failed", zap.Error(err))
				continue
			}

			if len(evicted) > 0 {
				zap.L().Warn("Evicted stale compute instances",
					zap.Strings("instance_ids", evicted),
					zap.Duration("threshold", threshold))
			}
		}
	}
}
