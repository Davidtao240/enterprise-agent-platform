-- M2-A: persist the usage payload carried by the run.succeeded Runtime event.
-- Additive, domain-neutral: agent_run_logs already stores usage_json; this
-- aligns agent_runs so both execution paths expose the same envelope fields.

ALTER TABLE agent_runs
ADD COLUMN IF NOT EXISTS usage_json JSONB;
