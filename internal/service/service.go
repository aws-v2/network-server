package service

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/internal/driver"
	"github.com/martin/network-service/internal/registry"
	"github.com/martin/network-service/internal/repository"
)

type NetworkService interface {
	CreateDefaultVPC(ctx context.Context, tenantID, tenantName string) (*domain.VPC, error)
	CreateVPC(ctx context.Context, tenantID, vpcName string) (*domain.VPC, error)

	// Elastic IP Management
	AllocateEIP(ctx context.Context) (*domain.ElasticIP, error)
	AssociateEIP(ctx context.Context, eipID, instanceID, privateIP string) error
	DisassociateEIP(ctx context.Context, eipID string) error
	AutoAssociateEIP(ctx context.Context, instanceID, privateIP string) (*domain.ElasticIP, error)

	// VPC Resolution & Validation
	GetDefaultVPC(ctx context.Context, tenantID string) (*domain.VPC, error)
	ValidateVPC(ctx context.Context, tenantID, vpcID string) (bool, string, string, error)
	ResolveDefaultNetwork(ctx context.Context, tenantID string) (string, string, string, error)

	// Resource Assignments (Legacy VPC-only - deprecated)
	AttachResourceToVPC(ctx context.Context, tenantID, resourceARN, vpcID string) error
	DetachResourceFromVPC(ctx context.Context, resourceARN string) error

	// Resource Networking (Subnet-level)
	AttachResourceToSubnet(ctx context.Context, tenantID, resourceARN, vpcID, subnetID string) (string, error)
	DetachResourceFromNetwork(ctx context.Context, resourceARN string) error
	ListResourcesInVPC(ctx context.Context, tenantID, vpcID string) ([]domain.ResourceNetworkAssignment, error)
	ResolveResourceNetwork(ctx context.Context, resourceARN string) (*domain.ResourceNetworkAssignment, error)

	// Reconciliation
	ReconcileVPCs(ctx context.Context) error

	// RDS Port Management
	ExposeRDSContainer(ctx context.Context, tenantID, resourceID, privateIP, publicIP string, privatePort int) (int, error)
	UnexposeRDSContainer(ctx context.Context, resourceID string) error
	ReconcileRDSPorts(ctx context.Context) error

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
	db                  *sqlx.DB
	vpcRepo             repository.VPCRepository
	subnetRepo          repository.SubnetRepository
	igwRepo             repository.InternetGatewayRepository
	rtRepo              repository.RouteTableRepository
	routeRepo           repository.RouteRepository
	sgRepo              repository.SecurityGroupRepository
	cidrRepo            repository.CIDRRepository
	eipRepo             repository.ElasticIPRepository
	resourceRepo        repository.ResourceVPCRepository
	netAssignRepo       repository.ResourceNetworkRepository
	rdsPortRepo         repository.RDSPortRepository
	computeReg          registry.ComputeRegistry
	bridgeDriver        driver.BridgeDriver
	iptablesDriver      driver.IptablesDriver
	routingDriver       driver.RoutingDriver
	dockerNetworkDriver driver.DockerNetworkDriver
	publicInterface     string
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
	dockerNetworkDriver driver.DockerNetworkDriver,
	resourceRepo repository.ResourceVPCRepository,
	netAssignRepo repository.ResourceNetworkRepository,
	rdsPortRepo repository.RDSPortRepository,
	computeReg registry.ComputeRegistry,
	publicInterface string,
) NetworkService {
	return &networkService{
		db:                  db,
		vpcRepo:             vpcRepo,
		subnetRepo:          subnetRepo,
		igwRepo:             igwRepo,
		rtRepo:              rtRepo,
		routeRepo:           routeRepo,
		sgRepo:              sgRepo,
		cidrRepo:            cidrRepo,
		eipRepo:             eipRepo,
		resourceRepo:        resourceRepo,
		netAssignRepo:       netAssignRepo,
		rdsPortRepo:         rdsPortRepo,
		computeReg:          computeReg,
		bridgeDriver:        bridgeDriver,
		iptablesDriver:      iptablesDriver,
		routingDriver:       routingDriver,
		dockerNetworkDriver: dockerNetworkDriver,
		publicInterface:     publicInterface,
	}
}

// DeriveGateway returns the gateway IP for a given private IP.
// By convention, VPC subnets are provisioned with the gateway at x.x.x.1,
// so 10.0.1.5 → gateway is 10.0.1.1
// func DeriveGateway(privateIP string) string {
// 	parts := strings.Split(privateIP, ".")
// 	if len(parts) != 4 {
// 		return ""
// 	}
// 	return fmt.Sprintf("%s.%s.%s.1", parts[0], parts[1], parts[2])
// }
