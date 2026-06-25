ALTER TABLE workflow_node_instances
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
