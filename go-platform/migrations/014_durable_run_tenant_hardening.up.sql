-- M1 tenant hardening: every durable Run relationship must preserve the
-- authenticated tenant boundary at the database level, not only in services.

CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_instances_tenant_id
ON workflow_instances (tenant_id, id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_nodes_workflow_id
ON workflow_node_instances (workflow_instance_id, id);

ALTER TABLE agent_threads
DROP CONSTRAINT IF EXISTS agent_threads_workflow_instance_id_fkey;

ALTER TABLE agent_threads
ADD CONSTRAINT agent_threads_tenant_workflow_fkey
FOREIGN KEY (tenant_id, workflow_instance_id)
REFERENCES workflow_instances(tenant_id, id);

ALTER TABLE agent_runs
DROP CONSTRAINT IF EXISTS agent_runs_workflow_instance_id_fkey,
DROP CONSTRAINT IF EXISTS agent_runs_node_instance_id_fkey,
DROP CONSTRAINT IF EXISTS agent_runs_parent_run_id_fkey;

ALTER TABLE agent_runs
ADD CONSTRAINT agent_runs_tenant_workflow_fkey
    FOREIGN KEY (tenant_id, workflow_instance_id)
    REFERENCES workflow_instances(tenant_id, id),
ADD CONSTRAINT agent_runs_workflow_node_fkey
    FOREIGN KEY (workflow_instance_id, node_instance_id)
    REFERENCES workflow_node_instances(workflow_instance_id, id),
ADD CONSTRAINT agent_runs_tenant_parent_fkey
    FOREIGN KEY (tenant_id, parent_run_id)
    REFERENCES agent_runs(tenant_id, id),
ADD CONSTRAINT agent_runs_node_requires_workflow_check
    CHECK (node_instance_id IS NULL OR workflow_instance_id IS NOT NULL);

ALTER TABLE agent_run_logs
DROP CONSTRAINT IF EXISTS agent_run_logs_durable_run_id_fkey;

ALTER TABLE agent_run_logs
ADD CONSTRAINT agent_run_logs_tenant_durable_run_fkey
FOREIGN KEY (tenant_id, durable_run_id)
REFERENCES agent_runs(tenant_id, id);
