-- Elastic IPs Table
CREATE TABLE IF NOT EXISTS elastic_ips (
    id TEXT PRIMARY KEY,
    public_ip TEXT NOT NULL UNIQUE,
    allocated BOOLEAN DEFAULT FALSE,
    instance_id TEXT UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for allocated IPs
CREATE INDEX IF NOT EXISTS idx_elastic_ips_allocated ON elastic_ips(allocated);
