-- VPCs Table
CREATE TABLE IF NOT EXISTS vpcs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    cidr_block TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Subnets Table
CREATE TABLE IF NOT EXISTS subnets (
    id TEXT PRIMARY KEY,
    vpc_id TEXT NOT NULL REFERENCES vpcs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    cidr_block TEXT NOT NULL,
    az TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Internet Gateways Table
CREATE TABLE IF NOT EXISTS internet_gateways (
    id TEXT PRIMARY KEY,
    vpc_id TEXT NOT NULL REFERENCES vpcs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Route Tables Table
CREATE TABLE IF NOT EXISTS route_tables (
    id TEXT PRIMARY KEY,
    vpc_id TEXT NOT NULL REFERENCES vpcs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Security Groups Table
CREATE TABLE IF NOT EXISTS security_groups (
    id TEXT PRIMARY KEY,
    vpc_id TEXT NOT NULL REFERENCES vpcs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexing for performance
CREATE INDEX IF NOT EXISTS idx_vpcs_tenant_id ON vpcs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_subnets_vpc_id ON subnets(vpc_id);
CREATE INDEX IF NOT EXISTS idx_internet_gateways_vpc_id ON internet_gateways(vpc_id);
CREATE INDEX IF NOT EXISTS idx_route_tables_vpc_id ON route_tables(vpc_id);
CREATE INDEX IF NOT EXISTS idx_security_groups_vpc_id ON security_groups(vpc_id);
