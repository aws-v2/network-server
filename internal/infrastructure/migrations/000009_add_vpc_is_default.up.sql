-- Add is_default column
ALTER TABLE vpcs ADD COLUMN IF NOT EXISTS is_default BOOLEAN NOT NULL DEFAULT false;

-- Add partial unique index to ensure only one default VPC per tenant
-- This works for both PostgreSQL and SQLite (3.9.0+)
CREATE UNIQUE INDEX IF NOT EXISTS idx_vpcs_tenant_default ON vpcs(tenant_id) WHERE is_default = true;
