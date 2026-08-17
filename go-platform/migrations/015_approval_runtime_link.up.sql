-- M1-C: link approval tasks to durable Runtime interrupts so an approval
-- decision can resume the paused Run. Additive, domain-neutral: only present
-- for agent_graph nodes whose Run entered waiting_human/waiting_external.

ALTER TABLE approval_tasks
ADD COLUMN IF NOT EXISTS durable_run_id UUID REFERENCES agent_runs(id);

ALTER TABLE approval_tasks
ADD COLUMN IF NOT EXISTS interrupt_id VARCHAR(128);

CREATE INDEX IF NOT EXISTS idx_approval_tasks_durable_run
ON approval_tasks (durable_run_id)
WHERE durable_run_id IS NOT NULL;
