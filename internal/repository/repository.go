package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/martin/network-service/internal/domain"
)

type VPCRepository interface {
	Create(ctx context.Context, vpc *domain.VPC) error
	CreateWithTx(ctx context.Context, tx *sqlx.Tx, vpc *domain.VPC) error
	GetByID(ctx context.Context, id string) (*domain.VPC, error)
	ListByTenant(ctx context.Context, tenantID string) ([]domain.VPC, error)
	UpdateStatus(ctx context.Context, id string, status string) error
	ListIncomplete(ctx context.Context) ([]domain.VPC, error)
	ListAll(ctx context.Context) ([]domain.VPC, error)
	GetDefaultVPC(ctx context.Context, tenantID string) (*domain.VPC, error)
}

type SubnetRepository interface {
	Create(ctx context.Context, subnet *domain.Subnet) error
	CreateWithTx(ctx context.Context, tx *sqlx.Tx, subnet *domain.Subnet) error
	GetByID(ctx context.Context, id string) (*domain.Subnet, error)
	ListByVPC(ctx context.Context, vpcID string) ([]domain.Subnet, error)
	AssociateRouteTable(ctx context.Context, subnetID, rtID string) error
	AssociateRouteTableWithTx(ctx context.Context, tx *sqlx.Tx, subnetID, rtID string) error
	ListByRouteTable(ctx context.Context, rtID string) ([]domain.Subnet, error)
}

type InternetGatewayRepository interface {
	Create(ctx context.Context, igw *domain.InternetGateway) error
	CreateWithTx(ctx context.Context, tx *sqlx.Tx, igw *domain.InternetGateway) error
	GetByID(ctx context.Context, id string) (*domain.InternetGateway, error)
	GetByVPCID(ctx context.Context, vpcID string) (*domain.InternetGateway, error)
}

type RouteTableRepository interface {
	Create(ctx context.Context, rt *domain.RouteTable) error
	CreateWithTx(ctx context.Context, tx *sqlx.Tx, rt *domain.RouteTable) error
	GetByID(ctx context.Context, id string) (*domain.RouteTable, error)
	ListByVPC(ctx context.Context, vpcID string) ([]domain.RouteTable, error)
	GetFullByID(ctx context.Context, id string) (*domain.RouteTable, error)
}

type RouteRepository interface {
	Create(ctx context.Context, route *domain.Route) error
	CreateWithTx(ctx context.Context, tx *sqlx.Tx, route *domain.Route) error
	ListByRouteTable(ctx context.Context, rtID string) ([]domain.Route, error)
	Delete(ctx context.Context, id string) error
}

type SecurityGroupRepository interface {
	Create(ctx context.Context, sg *domain.SecurityGroup) error
	CreateWithTx(ctx context.Context, tx *sqlx.Tx, sg *domain.SecurityGroup) error
	GetByID(ctx context.Context, id string) (*domain.SecurityGroup, error)
	ListByVPC(ctx context.Context, vpcID string) ([]domain.SecurityGroup, error)
}

type CIDRRepository interface {
	AllocateNext(ctx context.Context, tenantID string) (string, error)
	AllocateNextWithTx(ctx context.Context, tx *sqlx.Tx, tenantID string) (string, error)
	GetByTenant(ctx context.Context, tenantID string) (string, error)
}

type ElasticIPRepository interface {
	Create(ctx context.Context, eip *domain.ElasticIP) error
	CreateWithTx(ctx context.Context, tx *sqlx.Tx, eip *domain.ElasticIP) error
	GetByID(ctx context.Context, id string) (*domain.ElasticIP, error)
	GetByPublicIP(ctx context.Context, publicIP string) (*domain.ElasticIP, error)
	Update(ctx context.Context, eip *domain.ElasticIP) error
	UpdateWithTx(ctx context.Context, tx *sqlx.Tx, eip *domain.ElasticIP) error
	ListUnallocated(ctx context.Context) ([]domain.ElasticIP, error)
	GetByInstanceID(ctx context.Context, instanceID string) (*domain.ElasticIP, error)
}

type ResourceVPCRepository interface {
	Assign(ctx context.Context, tenantID, resourceARN, vpcID string) error
	GetByResource(ctx context.Context, resourceARN string) (string, error)
	Detach(ctx context.Context, resourceARN string) error
}

type ResourceNetworkRepository interface {
	AssignResource(ctx context.Context, tenantID, resourceARN, vpcID, subnetID, privateIP string) error
	GetByResource(ctx context.Context, resourceARN string) (*domain.ResourceNetworkAssignment, error)
	DetachResource(ctx context.Context, resourceARN string) error
	ListResourcesInVPC(ctx context.Context, vpcID string) ([]domain.ResourceNetworkAssignment, error)
	ListResourcesInSubnet(ctx context.Context, subnetID string) ([]domain.ResourceNetworkAssignment, error)
}

type postgresVPCRepository struct {
	db *sqlx.DB
}

func NewVPCRepository(db *sqlx.DB) VPCRepository {
	return &postgresVPCRepository{db: db}
}

func (r *postgresVPCRepository) Create(ctx context.Context, vpc *domain.VPC) error {
	return r.CreateWithTx(ctx, nil, vpc)
}

func (r *postgresVPCRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, vpc *domain.VPC) error {
	query := `INSERT INTO vpcs (id, name, cidr_block, bridge_name, tenant_id, status, is_default, created_at, updated_at) 
			  VALUES (:id, :name, :cidr_block, :bridge_name, :tenant_id, :status, :is_default, :created_at, :updated_at)`
	if tx != nil {
		_, err := tx.NamedExecContext(ctx, query, vpc)
		return err
	}
	_, err := r.db.NamedExecContext(ctx, query, vpc)
	return err
}

func (r *postgresVPCRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE vpcs SET status = $1, updated_at = $2 WHERE id = $3", status, time.Now(), id)
	return err
}

func (r *postgresVPCRepository) GetByID(ctx context.Context, id string) (*domain.VPC, error) {
	var vpc domain.VPC
	err := r.db.GetContext(ctx, &vpc, "SELECT * FROM vpcs WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	return &vpc, nil
}

func (r *postgresVPCRepository) ListByTenant(ctx context.Context, tenantID string) ([]domain.VPC, error) {
	var vpcs []domain.VPC
	err := r.db.SelectContext(ctx, &vpcs, "SELECT * FROM vpcs WHERE tenant_id = $1", tenantID)
	return vpcs, err
}

func (r *postgresVPCRepository) ListIncomplete(ctx context.Context) ([]domain.VPC, error) {
	var vpcs []domain.VPC
	err := r.db.SelectContext(ctx, &vpcs, "SELECT * FROM vpcs WHERE status != $1", domain.VPCStatusActive)
	return vpcs, err
}

func (r *postgresVPCRepository) ListAll(ctx context.Context) ([]domain.VPC, error) {
	var vpcs []domain.VPC
	err := r.db.SelectContext(ctx, &vpcs, "SELECT * FROM vpcs")
	return vpcs, err
}

func (r *postgresVPCRepository) GetDefaultVPC(ctx context.Context, tenantID string) (*domain.VPC, error) {
	var vpc domain.VPC
	err := r.db.GetContext(ctx, &vpc, "SELECT * FROM vpcs WHERE tenant_id = $1 AND is_default = true", tenantID)
	if err != nil {
		return nil, err
	}
	return &vpc, nil
}

type postgresSubnetRepository struct {
	db *sqlx.DB
}

func NewSubnetRepository(db *sqlx.DB) SubnetRepository {
	return &postgresSubnetRepository{db: db}
}

func (r *postgresSubnetRepository) Create(ctx context.Context, subnet *domain.Subnet) error {
	return r.CreateWithTx(ctx, nil, subnet)
}

func (r *postgresSubnetRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, subnet *domain.Subnet) error {
	query := `INSERT INTO subnets (id, vpc_id, route_table_id, name, cidr_block, az, created_at, updated_at) 
			  VALUES (:id, :vpc_id, :route_table_id, :name, :cidr_block, :az, :created_at, :updated_at)`
	if tx != nil {
		_, err := tx.NamedExecContext(ctx, query, subnet)
		return err
	}
	_, err := r.db.NamedExecContext(ctx, query, subnet)
	return err
}

func (r *postgresSubnetRepository) AssociateRouteTable(ctx context.Context, subnetID, rtID string) error {
	return r.AssociateRouteTableWithTx(ctx, nil, subnetID, rtID)
}

func (r *postgresSubnetRepository) AssociateRouteTableWithTx(ctx context.Context, tx *sqlx.Tx, subnetID, rtID string) error {
	query := "UPDATE subnets SET route_table_id = $1 WHERE id = $2"
	if tx != nil {
		_, err := tx.ExecContext(ctx, query, rtID, subnetID)
		return err
	}
	_, err := r.db.ExecContext(ctx, query, rtID, subnetID)
	return err
}

func (r *postgresSubnetRepository) ListByRouteTable(ctx context.Context, rtID string) ([]domain.Subnet, error) {
	var subnets []domain.Subnet
	err := r.db.SelectContext(ctx, &subnets, "SELECT * FROM subnets WHERE route_table_id = $1", rtID)
	return subnets, err
}

func (r *postgresSubnetRepository) ListByVPC(ctx context.Context, vpcID string) ([]domain.Subnet, error) {
	var subnets []domain.Subnet
	err := r.db.SelectContext(ctx, &subnets, "SELECT * FROM subnets WHERE vpc_id = $1", vpcID)
	return subnets, err
}

func (r *postgresSubnetRepository) GetByID(ctx context.Context, id string) (*domain.Subnet, error) {
	var subnet domain.Subnet
	err := r.db.GetContext(ctx, &subnet, "SELECT * FROM subnets WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	return &subnet, nil
}

type postgresInternetGatewayRepository struct {
	db *sqlx.DB
}

func NewInternetGatewayRepository(db *sqlx.DB) InternetGatewayRepository {
	return &postgresInternetGatewayRepository{db: db}
}

func (r *postgresInternetGatewayRepository) Create(ctx context.Context, igw *domain.InternetGateway) error {
	return r.CreateWithTx(ctx, nil, igw)
}

func (r *postgresInternetGatewayRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, igw *domain.InternetGateway) error {
	query := `INSERT INTO internet_gateways (id, vpc_id, name, created_at) 
			  VALUES (:id, :vpc_id, :name, :created_at)`
	if tx != nil {
		_, err := tx.NamedExecContext(ctx, query, igw)
		return err
	}
	_, err := r.db.NamedExecContext(ctx, query, igw)
	return err
}

func (r *postgresInternetGatewayRepository) GetByID(ctx context.Context, id string) (*domain.InternetGateway, error) {
	var igw domain.InternetGateway
	err := r.db.GetContext(ctx, &igw, "SELECT * FROM internet_gateways WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	return &igw, nil
}

func (r *postgresInternetGatewayRepository) GetByVPCID(ctx context.Context, vpcID string) (*domain.InternetGateway, error) {
	var igw domain.InternetGateway
	err := r.db.GetContext(ctx, &igw, "SELECT * FROM internet_gateways WHERE vpc_id = $1", vpcID)
	if err != nil {
		return nil, err
	}
	return &igw, nil
}

type postgresRouteTableRepository struct {
	db *sqlx.DB
}

func NewRouteTableRepository(db *sqlx.DB) RouteTableRepository {
	return &postgresRouteTableRepository{db: db}
}

func (r *postgresRouteTableRepository) Create(ctx context.Context, rt *domain.RouteTable) error {
	return r.CreateWithTx(ctx, nil, rt)
}

func (r *postgresRouteTableRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, rt *domain.RouteTable) error {
	query := `INSERT INTO route_tables (id, vpc_id, name, created_at) 
			  VALUES (:id, :vpc_id, :name, :created_at)`
	if tx != nil {
		_, err := tx.NamedExecContext(ctx, query, rt)
		return err
	}
	_, err := r.db.NamedExecContext(ctx, query, rt)
	return err
}

func (r *postgresRouteTableRepository) GetByID(ctx context.Context, id string) (*domain.RouteTable, error) {
	var rt domain.RouteTable
	err := r.db.GetContext(ctx, &rt, "SELECT * FROM route_tables WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

func (r *postgresRouteTableRepository) ListByVPC(ctx context.Context, vpcID string) ([]domain.RouteTable, error) {
	var rts []domain.RouteTable
	err := r.db.SelectContext(ctx, &rts, "SELECT * FROM route_tables WHERE vpc_id = $1", vpcID)
	return rts, err
}

func (r *postgresRouteTableRepository) GetFullByID(ctx context.Context, id string) (*domain.RouteTable, error) {
	var rt domain.RouteTable
	err := r.db.GetContext(ctx, &rt, "SELECT * FROM route_tables WHERE id = $1", id)
	if err != nil {
		return nil, err
	}

	var routes []domain.Route
	err = r.db.SelectContext(ctx, &routes, "SELECT * FROM routes WHERE route_table_id = $1", id)
	if err != nil {
		return nil, err
	}
	rt.Routes = routes
	return &rt, nil
}

type postgresRouteRepository struct {
	db *sqlx.DB
}

func NewRouteRepository(db *sqlx.DB) RouteRepository {
	return &postgresRouteRepository{db: db}
}

func (r *postgresRouteRepository) Create(ctx context.Context, route *domain.Route) error {
	return r.CreateWithTx(ctx, nil, route)
}

func (r *postgresRouteRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, route *domain.Route) error {
	query := `INSERT INTO routes (id, route_table_id, destination_cidr, target_type, target_id, created_at) 
			  VALUES (:id, :route_table_id, :destination_cidr, :target_type, :target_id, :created_at)`
	if tx != nil {
		_, err := tx.NamedExecContext(ctx, query, route)
		return err
	}
	_, err := r.db.NamedExecContext(ctx, query, route)
	return err
}

func (r *postgresRouteRepository) ListByRouteTable(ctx context.Context, rtID string) ([]domain.Route, error) {
	var routes []domain.Route
	err := r.db.SelectContext(ctx, &routes, "SELECT * FROM routes WHERE route_table_id = $1", rtID)
	return routes, err
}

func (r *postgresRouteRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM routes WHERE id = $1", id)
	return err
}

type postgresSecurityGroupRepository struct {
	db *sqlx.DB
}

func NewSecurityGroupRepository(db *sqlx.DB) SecurityGroupRepository {
	return &postgresSecurityGroupRepository{db: db}
}

func (r *postgresSecurityGroupRepository) Create(ctx context.Context, sg *domain.SecurityGroup) error {
	return r.CreateWithTx(ctx, nil, sg)
}

func (r *postgresSecurityGroupRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, sg *domain.SecurityGroup) error {
	query := `INSERT INTO security_groups (id, vpc_id, name, description, created_at) 
			  VALUES (:id, :vpc_id, :name, :description, :created_at)`
	if tx != nil {
		_, err := tx.NamedExecContext(ctx, query, sg)
		return err
	}
	_, err := r.db.NamedExecContext(ctx, query, sg)
	return err
}

func (r *postgresSecurityGroupRepository) GetByID(ctx context.Context, id string) (*domain.SecurityGroup, error) {
	var sg domain.SecurityGroup
	err := r.db.GetContext(ctx, &sg, "SELECT * FROM security_groups WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	return &sg, nil
}

func (r *postgresSecurityGroupRepository) ListByVPC(ctx context.Context, vpcID string) ([]domain.SecurityGroup, error) {
	var sgs []domain.SecurityGroup
	err := r.db.SelectContext(ctx, &sgs, "SELECT * FROM security_groups WHERE vpc_id = $1", vpcID)
	return sgs, err
}

type postgresCIDRRepository struct {
	db *sqlx.DB
}

func NewCIDRRepository(db *sqlx.DB) CIDRRepository {
	return &postgresCIDRRepository{db: db}
}

func (r *postgresCIDRRepository) AllocateNext(ctx context.Context, tenantID string) (string, error) {
	return r.AllocateNextWithTx(ctx, nil, tenantID)
}

func (r *postgresCIDRRepository) AllocateNextWithTx(ctx context.Context, tx *sqlx.Tx, tenantID string) (string, error) {
	// 1. Get next value from sequence
	var nextVal int
	seqQuery := "SELECT nextval('vpc_cidr_seq')"

	var err error
	if tx != nil {
		err = tx.GetContext(ctx, &nextVal, seqQuery)
	} else {
		err = r.db.GetContext(ctx, &nextVal, seqQuery)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get next CIDR sequence value: %w", err)
	}

	// 2. Construct CIDR string (10.<N>.0.0/16)
	cidr := fmt.Sprintf("10.%d.0.0/16", nextVal)

	// 3. Insert into allocation table
	insertQuery := "INSERT INTO allocated_vpc_cidrs (tenant_id, cidr_block) VALUES ($1, $2)"
	if tx != nil {
		_, err = tx.ExecContext(ctx, insertQuery, tenantID, cidr)
	} else {
		_, err = r.db.ExecContext(ctx, insertQuery, tenantID, cidr)
	}

	if err != nil {
		return "", fmt.Errorf("failed to record CIDR allocation: %w", err)
	}

	return cidr, nil
}

func (r *postgresCIDRRepository) GetByTenant(ctx context.Context, tenantID string) (string, error) {
	var cidr string
	err := r.db.GetContext(ctx, &cidr, "SELECT cidr_block FROM allocated_vpc_cidrs WHERE tenant_id = $1", tenantID)
	if err != nil {
		return "", err
	}
	return cidr, nil
}

type postgresElasticIPRepository struct {
	db *sqlx.DB
}

func NewElasticIPRepository(db *sqlx.DB) ElasticIPRepository {
	return &postgresElasticIPRepository{db: db}
}

func (r *postgresElasticIPRepository) Create(ctx context.Context, eip *domain.ElasticIP) error {
	return r.CreateWithTx(ctx, nil, eip)
}

func (r *postgresElasticIPRepository) CreateWithTx(ctx context.Context, tx *sqlx.Tx, eip *domain.ElasticIP) error {
	query := `INSERT INTO elastic_ips (id, public_ip, allocated, instance_id, private_ip, created_at, updated_at) 
			  VALUES (:id, :public_ip, :allocated, :instance_id, :private_ip, :created_at, :updated_at)`
	if tx != nil {
		_, err := tx.NamedExecContext(ctx, query, eip)
		return err
	}
	_, err := r.db.NamedExecContext(ctx, query, eip)
	return err
}

func (r *postgresElasticIPRepository) GetByID(ctx context.Context, id string) (*domain.ElasticIP, error) {
	var eip domain.ElasticIP
	err := r.db.GetContext(ctx, &eip, "SELECT * FROM elastic_ips WHERE id = $1", id)
	return &eip, err
}

func (r *postgresElasticIPRepository) GetByPublicIP(ctx context.Context, publicIP string) (*domain.ElasticIP, error) {
	var eip domain.ElasticIP
	err := r.db.GetContext(ctx, &eip, "SELECT * FROM elastic_ips WHERE public_ip = $1", publicIP)
	return &eip, err
}

func (r *postgresElasticIPRepository) Update(ctx context.Context, eip *domain.ElasticIP) error {
	return r.UpdateWithTx(ctx, nil, eip)
}

func (r *postgresElasticIPRepository) UpdateWithTx(ctx context.Context, tx *sqlx.Tx, eip *domain.ElasticIP) error {
	query := `UPDATE elastic_ips SET allocated = :allocated, instance_id = :instance_id, private_ip = :private_ip, updated_at = :updated_at WHERE id = :id`
	if tx != nil {
		_, err := tx.NamedExecContext(ctx, query, eip)
		return err
	}
	_, err := r.db.NamedExecContext(ctx, query, eip)
	return err
}

func (r *postgresElasticIPRepository) ListUnallocated(ctx context.Context) ([]domain.ElasticIP, error) {
	var eips []domain.ElasticIP
	err := r.db.SelectContext(ctx, &eips, "SELECT * FROM elastic_ips WHERE allocated = false")
	return eips, err
}

func (r *postgresElasticIPRepository) GetByInstanceID(ctx context.Context, instanceID string) (*domain.ElasticIP, error) {
	var eip domain.ElasticIP
	err := r.db.GetContext(ctx, &eip, "SELECT * FROM elastic_ips WHERE instance_id = $1", instanceID)
	return &eip, err
}

type postgresResourceVPCRepository struct {
	db *sqlx.DB
}

func NewResourceVPCRepository(db *sqlx.DB) ResourceVPCRepository {
	return &postgresResourceVPCRepository{db: db}
}

func (r *postgresResourceVPCRepository) Assign(ctx context.Context, tenantID, resourceARN, vpcID string) error {
	query := `INSERT INTO resource_vpc_assignments (resource_arn, tenant_id, vpc_id) 
			  VALUES ($1, $2, $3) 
			  ON CONFLICT (resource_arn) DO UPDATE SET vpc_id = EXCLUDED.vpc_id, assigned_at = CURRENT_TIMESTAMP`
	_, err := r.db.ExecContext(ctx, query, resourceARN, tenantID, vpcID)
	return err
}

func (r *postgresResourceVPCRepository) GetByResource(ctx context.Context, resourceARN string) (string, error) {
	var vpcID string
	err := r.db.GetContext(ctx, &vpcID, "SELECT vpc_id FROM resource_vpc_assignments WHERE resource_arn = $1", resourceARN)
	return vpcID, err
}

func (r *postgresResourceVPCRepository) Detach(ctx context.Context, resourceARN string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM resource_vpc_assignments WHERE resource_arn = $1", resourceARN)
	return err
}

type postgresResourceNetworkRepository struct {
	db *sqlx.DB
}

func NewResourceNetworkRepository(db *sqlx.DB) ResourceNetworkRepository {
	return &postgresResourceNetworkRepository{db: db}
}

func (r *postgresResourceNetworkRepository) AssignResource(ctx context.Context, tenantID, resourceARN, vpcID, subnetID, privateIP string) error {
	query := `INSERT INTO resource_network_assignments (resource_arn, tenant_id, vpc_id, subnet_id, private_ip) 
			  VALUES ($1, $2, $3, $4, $5) 
			  ON CONFLICT (resource_arn) DO UPDATE SET 
			  vpc_id = EXCLUDED.vpc_id, 
			  subnet_id = EXCLUDED.subnet_id, 
			  private_ip = EXCLUDED.private_ip, 
			  assigned_at = CURRENT_TIMESTAMP`
	_, err := r.db.ExecContext(ctx, query, resourceARN, tenantID, vpcID, subnetID, privateIP)
	return err
}

func (r *postgresResourceNetworkRepository) GetByResource(ctx context.Context, resourceARN string) (*domain.ResourceNetworkAssignment, error) {
	var assignment domain.ResourceNetworkAssignment
	err := r.db.GetContext(ctx, &assignment, "SELECT * FROM resource_network_assignments WHERE resource_arn = $1", resourceARN)
	if err != nil {
		return nil, err
	}
	return &assignment, nil
}

func (r *postgresResourceNetworkRepository) DetachResource(ctx context.Context, resourceARN string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM resource_network_assignments WHERE resource_arn = $1", resourceARN)
	return err
}

func (r *postgresResourceNetworkRepository) ListResourcesInVPC(ctx context.Context, vpcID string) ([]domain.ResourceNetworkAssignment, error) {
	var assignments []domain.ResourceNetworkAssignment
	err := r.db.SelectContext(ctx, &assignments, "SELECT * FROM resource_network_assignments WHERE vpc_id = $1", vpcID)
	return assignments, err
}

func (r *postgresResourceNetworkRepository) ListResourcesInSubnet(ctx context.Context, subnetID string) ([]domain.ResourceNetworkAssignment, error) {
	var assignments []domain.ResourceNetworkAssignment
	err := r.db.SelectContext(ctx, &assignments, "SELECT * FROM resource_network_assignments WHERE subnet_id = $1", subnetID)
	return assignments, err
}
