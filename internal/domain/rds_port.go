package domain

import "time"

// RDSPortAllocation represents a public port allocation for an RDS database
type RDSPortAllocation struct {
	ID          string     `db:"id" json:"id"`
	TenantID    string     `db:"tenant_id" json:"tenant_id"`
	ResourceID  string     `db:"resource_id" json:"resource_id"`
	PrivateIP   string     `db:"private_ip" json:"private_ip"`
	PrivatePort int        `db:"private_port" json:"private_port"`
	PublicIP    string     `db:"public_ip" json:"public_ip"`
	PublicPort  int        `db:"public_port" json:"public_port"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	ReleasedAt  *time.Time `db:"released_at" json:"released_at"`
}
