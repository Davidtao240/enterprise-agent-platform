-- V1.3 compatibility: auth permission queries filter permissions.deleted_at.
ALTER TABLE permissions ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
