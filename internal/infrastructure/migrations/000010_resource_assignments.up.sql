-- Table to track resource assignments to VPCs
CREATE TABLE IF NOT EXISTS resource_vpc_assignments (
    resource_arn TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    vpc_id TEXT NOT NULL REFERENCES vpcs(id) ON DELETE CASCADE,
    assigned_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for tenant resource listing
CREATE INDEX IF NOT EXISTS idx_resource_vpc_tenant ON resource_vpc_assignments(tenant_id);
-- Index for vpc resource listing
CREATE INDEX IF NOT EXISTS idx_resource_vpc_vpc ON resource_vpc_assignments(vpc_id);
