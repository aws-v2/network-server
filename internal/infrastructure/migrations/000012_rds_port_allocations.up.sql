CREATE TABLE rds_port_allocations (
    id UUID PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    resource_id VARCHAR(255) NOT NULL,
    private_ip VARCHAR(255) NOT NULL,
    private_port INT NOT NULL,
    public_ip VARCHAR(255) NOT NULL,
    public_port INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    released_at TIMESTAMP NULL
);

-- Index for finding active allocation for a resource quickly
CREATE INDEX idx_rds_ports_resource ON rds_port_allocations (resource_id);
-- Index for finding available ports (where released_at is NULL)
CREATE INDEX idx_rds_ports_active ON rds_port_allocations (released_at) WHERE released_at IS NULL;
