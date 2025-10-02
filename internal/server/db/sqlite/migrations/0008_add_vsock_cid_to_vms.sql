-- Add vsock_cid column for vsock communication
-- Each VM gets a unique CID for vsock-only networking
ALTER TABLE vms ADD COLUMN vsock_cid INTEGER NOT NULL DEFAULT 0;

-- Create index for efficient CID lookups
CREATE INDEX IF NOT EXISTS idx_vms_vsock_cid ON vms(vsock_cid);
