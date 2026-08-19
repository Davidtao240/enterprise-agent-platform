-- AVR Telemetry: Add fields to agent_run_logs for tracking agent independent output time
ALTER TABLE agent_run_logs ADD COLUMN IF NOT EXISTS human_interaction_ms INT NOT NULL DEFAULT 0;
ALTER TABLE agent_run_logs ADD COLUMN IF NOT EXISTS clarification_count INT NOT NULL DEFAULT 0;
ALTER TABLE agent_run_logs ADD COLUMN IF NOT EXISTS rework_count INT NOT NULL DEFAULT 0;
COMMENT ON COLUMN agent_run_logs.human_interaction_ms IS 'Milliseconds spent waiting for or processing human input (clarifications, approvals)';
COMMENT ON COLUMN agent_run_logs.clarification_count IS 'Number of clarification rounds triggered during this agent run';
COMMENT ON COLUMN agent_run_logs.rework_count IS 'Number of times this agent run was retried due to correction or rework';

-- AVR Telemetry: Add conversation-level timing
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS total_duration_ms INT;
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS active_duration_ms INT;
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS message_count INT NOT NULL DEFAULT 0;
COMMENT ON COLUMN conversations.total_duration_ms IS 'Total wall-clock duration from first message to conversation close (ms)';
COMMENT ON COLUMN conversations.active_duration_ms IS 'Time the conversation was actively being worked on (excluding idle waits) (ms)';

-- AVR Telemetry: Add approval timing
ALTER TABLE approval_tasks ADD COLUMN IF NOT EXISTS duration_ms INT;
COMMENT ON COLUMN approval_tasks.duration_ms IS 'Time from task creation to decision (approve/reject) in milliseconds';

-- Create AVR daily summary table (materialized view pattern)
CREATE TABLE IF NOT EXISTS avr_daily_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    metric_date DATE NOT NULL,
    agent_independent_ms BIGINT NOT NULL DEFAULT 0,
    conversation_total_ms BIGINT NOT NULL DEFAULT 0,
    conversation_active_ms BIGINT NOT NULL DEFAULT 0,
    approval_avg_ms BIGINT NOT NULL DEFAULT 0,
    rework_total INT NOT NULL DEFAULT 0,
    total_runs INT NOT NULL DEFAULT 0,
    total_conversations INT NOT NULL DEFAULT 0,
    total_approvals INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, metric_date)
);
CREATE INDEX IF NOT EXISTS idx_avr_daily_tenant_date ON avr_daily_metrics(tenant_id, metric_date DESC);