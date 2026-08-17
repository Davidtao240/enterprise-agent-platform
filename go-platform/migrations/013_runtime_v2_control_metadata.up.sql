-- M1-B: Go control-plane metadata for Python checkpoints and interrupts.
-- Graph state remains owned by the Python checkpointer; these rows are only
-- tenant-scoped indexes and recovery/audit references.

CREATE TABLE IF NOT EXISTS agent_checkpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    run_id UUID NOT NULL,
    version BIGINT NOT NULL CHECK (version > 0),
    backend VARCHAR(32) NOT NULL,
    checkpoint_ref TEXT NOT NULL,
    state_hash VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, run_id) REFERENCES agent_runs(tenant_id, id),
    UNIQUE (run_id, version)
);

CREATE INDEX IF NOT EXISTS idx_agent_checkpoints_tenant_run
ON agent_checkpoints (tenant_id, run_id, version DESC);

CREATE TABLE IF NOT EXISTS agent_interrupts (
    id VARCHAR(128) PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    run_id UUID NOT NULL,
    step_id UUID REFERENCES agent_run_steps(id),
    checkpoint_version BIGINT NOT NULL CHECK (checkpoint_version > 0),
    kind VARCHAR(32) NOT NULL
        CHECK (kind IN ('human', 'external', 'input_required')),
    status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'resumed', 'cancelled', 'expired')),
    resume_schema_json JSONB NOT NULL DEFAULT '{}',
    expires_at TIMESTAMPTZ,
    resumed_by UUID REFERENCES users(id),
    resume_idempotency_key VARCHAR(255),
    resumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, run_id) REFERENCES agent_runs(tenant_id, id),
    UNIQUE (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS idx_agent_interrupts_tenant_run_status
ON agent_interrupts (tenant_id, run_id, status, created_at DESC);
