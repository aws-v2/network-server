package domain

import "time"

const (
	VPCStatusProvisioning = "provisioning"
	VPCStatusActive       = "active"
	VPCStatusError        = "error"
)

type VPC struct {
	ID         string    `db:"id" json:"id"`
	Name       string    `db:"name" json:"name"`
	CIDRBlock  string    `db:"cidr_block" json:"cidr_block"`
	BridgeName string    `db:"bridge_name" json:"bridge_name"`
	TenantID   string    `db:"tenant_id" json:"tenant_id"`
	Status     string    `db:"status" json:"status"`
	IsDefault  bool      `db:"is_default" json:"is_default"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time `db:"updated_at" json:"updated_at"`
}

type Subnet struct {
	ID           string    `db:"id" json:"id"`
	VPCID        string    `db:"vpc_id" json:"vpc_id"`
	RouteTableID string    `db:"route_table_id" json:"route_table_id"`
	Name         string    `db:"name" json:"name"`
	CIDRBlock    string    `db:"cidr_block" json:"cidr_block"`
	Az           string    `db:"az" json:"az"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

type InternetGateway struct {
	ID        string    `db:"id" json:"id"`
	VPCID     string    `db:"vpc_id" json:"vpc_id"`
	Name      string    `db:"name" json:"name"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type RouteTable struct {
	ID        string    `db:"id" json:"id"`
	VPCID     string    `db:"vpc_id" json:"vpc_id"`
	Name      string    `db:"name" json:"name"`
	Routes    []Route   `json:"routes,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Route struct {
	ID              string    `db:"id" json:"id"`
	RouteTableID    string    `db:"route_table_id" json:"route_table_id"`
	DestinationCIDR string    `db:"destination_cidr" json:"destination_cidr"`
	TargetType      string    `db:"target_type" json:"target_type"` // igw, nat, instance, vpc_peering
	TargetID        string    `db:"target_id" json:"target_id"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
}

type SecurityGroup struct {
	ID          string    `db:"id" json:"id"`
	VPCID       string    `db:"vpc_id" json:"vpc_id"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

type ElasticIP struct {
	ID         string    `db:"id" json:"id"`
	PublicIP   string    `db:"public_ip" json:"public_ip"`
	Allocated  bool      `db:"allocated" json:"allocated"`
	InstanceID string    `db:"instance_id" json:"instance_id"`
	PrivateIP  string    `db:"private_ip" json:"private_ip"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time `db:"updated_at" json:"updated_at"`
}
