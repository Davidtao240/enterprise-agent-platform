-- V2.2: prevent duplicate workflow creation during client or network retries.
ALTER TABLE workflow_instances ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(128);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_instances_idempotency
ON workflow_instances (created_by, idempotency_key)
WHERE idempotency_key IS NOT NULL;
