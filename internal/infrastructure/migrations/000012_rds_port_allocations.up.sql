CREATE TABLE rds_port_allocations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     VARCHAR NOT NULL,
    resource_id   VARCHAR NOT NULL UNIQUE,
    private_ip    VARCHAR NOT NULL,
    private_port  INT NOT NULL DEFAULT 5432,
    public_ip     VARCHAR NOT NULL,
    public_port   INT NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    released_at   TIMESTAMP NULL
);

CREATE UNIQUE INDEX idx_rds_active_port 
    ON rds_port_allocations(public_port) 
    WHERE released_at IS NULL;
