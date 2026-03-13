package service

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/martin/network-service/internal/driver"
	"github.com/martin/network-service/internal/registry"
	"github.com/martin/network-service/internal/repository"
)

type ServiceMocks struct {
	VPCRepo             *repository.MockVPCRepository
	SubnetRepo          *repository.MockSubnetRepository
	IGWRepo             *repository.MockInternetGatewayRepository
	RTRepo              *repository.MockRouteTableRepository
	RouteRepo           *repository.MockRouteRepository
	SGRepo              *repository.MockSecurityGroupRepository
	CIDRRepo            *repository.MockCIDRRepository
	EIPRepo             *repository.MockElasticIPRepository
	ComputeReg          *registry.MockComputeRegistry
	BridgeDriver        *driver.MockBridgeDriver
	IptablesDriver      *driver.MockIptablesDriver
	DockerNetworkDriver *driver.MockDockerNetworkDriver
	DBMock              sqlmock.Sqlmock
}

func NewTestService() (NetworkService, *ServiceMocks) {
	db, dbMock, _ := sqlmock.New()
	sqlxDB := sqlx.NewDb(db, "sqlmock")

	mocks := &ServiceMocks{
		VPCRepo:             new(repository.MockVPCRepository),
		SubnetRepo:          new(repository.MockSubnetRepository),
		IGWRepo:             new(repository.MockInternetGatewayRepository),
		RTRepo:              new(repository.MockRouteTableRepository),
		RouteRepo:           new(repository.MockRouteRepository),
		SGRepo:              new(repository.MockSecurityGroupRepository),
		CIDRRepo:            new(repository.MockCIDRRepository),
		EIPRepo:             new(repository.MockElasticIPRepository),
		ComputeReg:          new(registry.MockComputeRegistry),
		BridgeDriver:        new(driver.MockBridgeDriver),
		IptablesDriver:      new(driver.MockIptablesDriver),
		DockerNetworkDriver: new(driver.MockDockerNetworkDriver),
		DBMock:              dbMock,
	}

	svc := &networkService{
		db:                  sqlxDB,
		vpcRepo:             mocks.VPCRepo,
		subnetRepo:          mocks.SubnetRepo,
		igwRepo:             mocks.IGWRepo,
		rtRepo:              mocks.RTRepo,
		routeRepo:           mocks.RouteRepo,
		sgRepo:              mocks.SGRepo,
		cidrRepo:            mocks.CIDRRepo,
		eipRepo:             mocks.EIPRepo,
		computeReg:          mocks.ComputeReg,
		bridgeDriver:        mocks.BridgeDriver,
		iptablesDriver:      mocks.IptablesDriver,
		dockerNetworkDriver: mocks.DockerNetworkDriver,
	}

	return svc, mocks
}
