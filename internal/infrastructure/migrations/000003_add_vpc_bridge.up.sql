 

-- Add bridge_name column to vpcs table
ALTER TABLE vpcs ADD COLUMN IF NOT EXISTS bridge_name TEXT;

-- Update existing vpcs (if any) with a placeholder or legacy name.....
UPDATE vpcs SET bridge_name = 'br-vpc-' || substr(id, 1, 8) WHERE bridge_name IS NULL;
