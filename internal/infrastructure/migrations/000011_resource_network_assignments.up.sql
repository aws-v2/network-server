-- Migration: 000011_resource_network_assignments.up.sql
CREATE TABLE IF NOT EXISTS resource_network_assignments (
    resource_arn TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    vpc_id TEXT NOT NULL REFERENCES vpcs(id) ON DELETE CASCADE,
    subnet_id TEXT NOT NULL REFERENCES subnets(id) ON DELETE CASCADE,
    private_ip TEXT,
    assigned_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_resource_network_vpc
ON resource_network_assignments(vpc_id);

CREATE INDEX IF NOT EXISTS idx_resource_network_subnet
ON resource_network_assignments(subnet_id);

CREATE INDEX IF NOT EXISTS idx_resource_network_tenant
ON resource_network_assignments(tenant_id);
