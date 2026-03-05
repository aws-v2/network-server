-- Allocated VPC CIDRs Table
CREATE TABLE IF NOT EXISTS allocated_vpc_cidrs (
    tenant_id TEXT PRIMARY KEY,
    cidr_block TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for unique CIDRs
CREATE INDEX IF NOT EXISTS idx_allocated_cidrs_block ON allocated_vpc_cidrs(cidr_block);
