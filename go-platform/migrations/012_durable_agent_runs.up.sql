-- M1-A: durable Agent Run control-plane index and V1 compatibility mapping.
-- These tables are business-domain neutral. Python checkpoint state remains out of scope.

CREATE TABLE IF NOT EXISTS agent_threads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    created_by UUID NOT NULL REFERENCES users(id),
    business_app_code VARCHAR(64),
    workflow_instance_id UUID REFERENCES workflow_instances(id),
    title VARCHAR(255),
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'closed', 'deleted')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_threads_workflow
ON agent_threads (tenant_id, workflow_instance_id)
WHERE workflow_instance_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_agent_threads_tenant_status
ON agent_threads (tenant_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS agent_runs (
    id UUID PRIMARY KEY,
    thread_id UUID NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    trace_id VARCHAR(128) NOT NULL,
    workflow_instance_id UUID REFERENCES workflow_instances(id),
    node_instance_id UUID REFERENCES workflow_node_instances(id),
    parent_run_id UUID REFERENCES agent_runs(id),
    graph_key VARCHAR(128) NOT NULL,
    graph_version VARCHAR(32) NOT NULL,
    configuration_snapshot_json JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(32) NOT NULL
        CHECK (status IN ('queued', 'running', 'waiting_human', 'waiting_external', 'succeeded', 'failed', 'cancelled')),
    attempt INT NOT NULL CHECK (attempt > 0),
    checkpoint_version BIGINT CHECK (checkpoint_version IS NULL OR checkpoint_version >= 0),
    lease_owner VARCHAR(128),
    lease_expires_at TIMESTAMPTZ,
    heartbeat_at TIMESTAMPTZ,
    deadline_at TIMESTAMPTZ,
    budget_json JSONB,
    output_summary_json JSONB,
    error_json JSONB,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, thread_id) REFERENCES agent_threads(tenant_id, id),
    UNIQUE (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS idx_agent_runs_tenant_status
ON agent_runs (tenant_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_agent_runs_workflow_node
ON agent_runs (workflow_instance_id, node_instance_id, attempt DESC);

CREATE INDEX IF NOT EXISTS idx_agent_runs_trace
ON agent_runs (trace_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_runs_node_attempt
ON agent_runs (tenant_id, node_instance_id, attempt)
WHERE node_instance_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS agent_run_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    run_id UUID NOT NULL,
    sequence BIGINT NOT NULL CHECK (sequence > 0),
    attempt INT NOT NULL CHECK (attempt > 0),
    step_type VARCHAR(32) NOT NULL
        CHECK (step_type IN ('model', 'tool', 'checkpoint', 'interrupt', 'system')),
    name VARCHAR(128),
    status VARCHAR(32) NOT NULL
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'cancelled')),
    input_summary_json JSONB,
    output_summary_json JSONB,
    usage_json JSONB,
    error_json JSONB,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, run_id) REFERENCES agent_runs(tenant_id, id),
    UNIQUE (run_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_agent_run_steps_tenant_run
ON agent_run_steps (tenant_id, run_id, sequence);

CREATE TABLE IF NOT EXISTS runtime_events (
    event_id VARCHAR(128) PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    run_id UUID NOT NULL,
    sequence BIGINT NOT NULL CHECK (sequence > 0),
    attempt INT NOT NULL CHECK (attempt > 0),
    event_type VARCHAR(64) NOT NULL,
    payload_json JSONB,
    checkpoint_version BIGINT CHECK (checkpoint_version IS NULL OR checkpoint_version >= 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, run_id) REFERENCES agent_runs(tenant_id, id),
    UNIQUE (run_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_runtime_events_tenant_run
ON runtime_events (tenant_id, run_id, sequence);

ALTER TABLE agent_run_logs
ADD COLUMN IF NOT EXISTS durable_run_id UUID REFERENCES agent_runs(id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_run_logs_durable_run
ON agent_run_logs (durable_run_id)
WHERE durable_run_id IS NOT NULL;
