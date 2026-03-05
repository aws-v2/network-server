-- Routes Table
CREATE TABLE IF NOT EXISTS routes (
    id TEXT PRIMARY KEY,
    route_table_id TEXT NOT NULL REFERENCES route_tables(id) ON DELETE CASCADE,
    destination_cidr TEXT NOT NULL,
    target_type TEXT NOT NULL, -- 'igw', 'nat', 'instance', 'vpc_peering'
    target_id TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Add route_table_id to subnets for explicit association
-- We don't make it NOT NULL yet to allow migration of existing subnets
ALTER TABLE subnets ADD COLUMN IF NOT EXISTS route_table_id TEXT REFERENCES route_tables(id) ON DELETE SET NULL;

-- Indexing for performance
CREATE INDEX IF NOT EXISTS idx_routes_table_id ON routes(route_table_id);
CREATE INDEX IF NOT EXISTS idx_subnets_route_table_id ON subnets(route_table_id);
