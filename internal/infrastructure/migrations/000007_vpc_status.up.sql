-- Add status column to vpcs table
ALTER TABLE vpcs ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'provisioning';

-- Update existing VPCs to active (if any)
UPDATE vpcs SET status = 'active';
