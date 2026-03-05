package domain

import "time"

type ResourceNetworkAssignment struct {
	ResourceARN string    `db:"resource_arn" json:"resource_arn"`
	TenantID    string    `db:"tenant_id" json:"tenant_id"`
	VPCID       string    `db:"vpc_id" json:"vpc_id"`
	SubnetID    string    `db:"subnet_id" json:"subnet_id"`
	PrivateIP   string    `db:"private_ip" json:"private_ip"`
	AssignedAt  time.Time `db:"assigned_at" json:"assigned_at"`
}
