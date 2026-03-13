package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/martin/network-service/internal/domain"
	"github.com/stretchr/testify/mock"
)

type MockVPCRepository struct {
	mock.Mock
}

func (m *MockVPCRepository) Create(ctx context.Context, vpc *domain.VPC) error {
	args := m.Called(ctx, vpc)
	return args.Error(0)
}

func (m *MockVPCRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, vpc *domain.VPC) error {
	args := m.Called(ctx, tx, vpc)
	return args.Error(0)
}

func (m *MockVPCRepository) GetByID(ctx context.Context, id string) (*domain.VPC, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.VPC), args.Error(1)
}

func (m *MockVPCRepository) ListByTenant(ctx context.Context, tenantID string) ([]domain.VPC, error) {
	args := m.Called(ctx, tenantID)
	return args.Get(0).([]domain.VPC), args.Error(1)
}

func (m *MockVPCRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockVPCRepository) ListIncomplete(ctx context.Context) ([]domain.VPC, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.VPC), args.Error(1)
}

func (m *MockVPCRepository) ListAll(ctx context.Context) ([]domain.VPC, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.VPC), args.Error(1)
}

func (m *MockVPCRepository) GetDefaultVPC(ctx context.Context, tenantID string) (*domain.VPC, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.VPC), args.Error(1)
}

type MockSubnetRepository struct {
	mock.Mock
}

func (m *MockSubnetRepository) Create(ctx context.Context, subnet *domain.Subnet) error {
	args := m.Called(ctx, subnet)
	return args.Error(0)
}

func (m *MockSubnetRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, subnet *domain.Subnet) error {
	args := m.Called(ctx, tx, subnet)
	return args.Error(0)
}

func (m *MockSubnetRepository) GetByID(ctx context.Context, id string) (*domain.Subnet, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Subnet), args.Error(1)
}

func (m *MockSubnetRepository) ListByVPC(ctx context.Context, vpcID string) ([]domain.Subnet, error) {
	args := m.Called(ctx, vpcID)
	return args.Get(0).([]domain.Subnet), args.Error(1)
}

func (m *MockSubnetRepository) AssociateRouteTable(ctx context.Context, subnetID, rtID string) error {
	args := m.Called(ctx, subnetID, rtID)
	return args.Error(0)
}

func (m *MockSubnetRepository) AssociateRouteTableWithTx(ctx context.Context, tx *sqlx.Tx, subnetID, rtID string) error {
	args := m.Called(ctx, tx, subnetID, rtID)
	return args.Error(0)
}

func (m *MockSubnetRepository) ListByRouteTable(ctx context.Context, rtID string) ([]domain.Subnet, error) {
	args := m.Called(ctx, rtID)
	return args.Get(0).([]domain.Subnet), args.Error(1)
}

type MockCIDRRepository struct {
	mock.Mock
}

func (m *MockCIDRRepository) AllocateNext(ctx context.Context, tenantID string) (string, error) {
	args := m.Called(ctx, tenantID)
	return args.String(0), args.Error(1)
}

func (m *MockCIDRRepository) AllocateNextWithTx(ctx context.Context, tx *sqlx.Tx, tenantID string) (string, error) {
	args := m.Called(ctx, tx, tenantID)
	return args.String(0), args.Error(1)
}

func (m *MockCIDRRepository) GetByTenant(ctx context.Context, tenantID string) (string, error) {
	args := m.Called(ctx, tenantID)
	return args.String(0), args.Error(1)
}

type MockRouteTableRepository struct {
	mock.Mock
}

func (m *MockRouteTableRepository) Create(ctx context.Context, rt *domain.RouteTable) error {
	args := m.Called(ctx, rt)
	return args.Error(0)
}

func (m *MockRouteTableRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, rt *domain.RouteTable) error {
	args := m.Called(ctx, tx, rt)
	return args.Error(0)
}

func (m *MockRouteTableRepository) GetByID(ctx context.Context, id string) (*domain.RouteTable, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RouteTable), args.Error(1)
}

func (m *MockRouteTableRepository) ListByVPC(ctx context.Context, vpcID string) ([]domain.RouteTable, error) {
	args := m.Called(ctx, vpcID)
	return args.Get(0).([]domain.RouteTable), args.Error(1)
}

func (m *MockRouteTableRepository) GetFullByID(ctx context.Context, id string) (*domain.RouteTable, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RouteTable), args.Error(1)
}

type MockInternetGatewayRepository struct {
	mock.Mock
}

func (m *MockInternetGatewayRepository) Create(ctx context.Context, igw *domain.InternetGateway) error {
	args := m.Called(ctx, igw)
	return args.Error(0)
}

func (m *MockInternetGatewayRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, igw *domain.InternetGateway) error {
	args := m.Called(ctx, tx, igw)
	return args.Error(0)
}

func (m *MockInternetGatewayRepository) GetByID(ctx context.Context, id string) (*domain.InternetGateway, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.InternetGateway), args.Error(1)
}

func (m *MockInternetGatewayRepository) GetByVPCID(ctx context.Context, vpcID string) (*domain.InternetGateway, error) {
	args := m.Called(ctx, vpcID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.InternetGateway), args.Error(1)
}

type MockRouteRepository struct {
	mock.Mock
}

func (m *MockRouteRepository) Create(ctx context.Context, route *domain.Route) error {
	args := m.Called(ctx, route)
	return args.Error(0)
}

func (m *MockRouteRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, route *domain.Route) error {
	args := m.Called(ctx, tx, route)
	return args.Error(0)
}

func (m *MockRouteRepository) ListByRouteTable(ctx context.Context, rtID string) ([]domain.Route, error) {
	args := m.Called(ctx, rtID)
	return args.Get(0).([]domain.Route), args.Error(1)
}

func (m *MockRouteRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

type MockSecurityGroupRepository struct {
	mock.Mock
}

func (m *MockSecurityGroupRepository) Create(ctx context.Context, sg *domain.SecurityGroup) error {
	args := m.Called(ctx, sg)
	return args.Error(0)
}

func (m *MockSecurityGroupRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, sg *domain.SecurityGroup) error {
	args := m.Called(ctx, tx, sg)
	return args.Error(0)
}

func (m *MockSecurityGroupRepository) GetByID(ctx context.Context, id string) (*domain.SecurityGroup, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SecurityGroup), args.Error(1)
}

func (m *MockSecurityGroupRepository) ListByVPC(ctx context.Context, vpcID string) ([]domain.SecurityGroup, error) {
	args := m.Called(ctx, vpcID)
	return args.Get(0).([]domain.SecurityGroup), args.Error(1)
}

type MockElasticIPRepository struct {
	mock.Mock
}

func (m *MockElasticIPRepository) Create(ctx context.Context, eip *domain.ElasticIP) error {
	args := m.Called(ctx, eip)
	return args.Error(0)
}

func (m *MockElasticIPRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, eip *domain.ElasticIP) error {
	args := m.Called(ctx, tx, eip)
	return args.Error(0)
}

func (m *MockElasticIPRepository) GetByID(ctx context.Context, id string) (*domain.ElasticIP, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ElasticIP), args.Error(1)
}

func (m *MockElasticIPRepository) GetByPublicIP(ctx context.Context, publicIP string) (*domain.ElasticIP, error) {
	args := m.Called(ctx, publicIP)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ElasticIP), args.Error(1)
}

func (m *MockElasticIPRepository) Update(ctx context.Context, eip *domain.ElasticIP) error {
	args := m.Called(ctx, eip)
	return args.Error(0)
}

func (m *MockElasticIPRepository) UpdateWithTx(ctx context.Context, tx *sqlx.Tx, eip *domain.ElasticIP) error {
	args := m.Called(ctx, tx, eip)
	return args.Error(0)
}

func (m *MockElasticIPRepository) ListUnallocated(ctx context.Context) ([]domain.ElasticIP, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.ElasticIP), args.Error(1)
}

func (m *MockElasticIPRepository) GetByInstanceID(ctx context.Context, instanceID string) (*domain.ElasticIP, error) {
	args := m.Called(ctx, instanceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ElasticIP), args.Error(1)
}
