-- Add private_ip column to elastic_ips
ALTER TABLE elastic_ips ADD COLUMN IF NOT EXISTS private_ip TEXT;
