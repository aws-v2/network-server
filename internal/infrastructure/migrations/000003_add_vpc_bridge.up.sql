 

    -- Update existing vpcs (if any) with a placeholder or legacy name
UPDATE vpcs SET bridge_name = 'br-vpc-' || substr(id, 1, 8) WHERE bridge_name IS NULL;
