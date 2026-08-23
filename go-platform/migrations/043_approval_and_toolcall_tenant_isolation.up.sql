-- migrations/043_approval_and_toolcall_tenant_isolation.up.sql
-- P0 修复:审批任务与 Tool Call 租户隔离
-- 1) approval_tasks 增加 tenant_id(从 workflow_instances 回填;tool_call 审批从 tool_calls 回填)
-- 2) tool_calls 增加 tenant_id(新建 Tool Call 由可信上下文注入)

-- ── approval_tasks ──
ALTER TABLE approval_tasks ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id);

-- 工作流审批:从 workflow_instances 继承租户
UPDATE approval_tasks at
SET tenant_id = wi.tenant_id
FROM workflow_instances wi
WHERE at.workflow_instance_id = wi.id AND at.tenant_id IS NULL;

-- Tool Call 审批:先回填 tool_calls 租户,再继承(见下方 tool_calls 段)
ALTER TABLE tool_calls ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id);

-- 历史 Tool Call 无租户上下文,归入默认租户
UPDATE tool_calls SET tenant_id = '00000000-0000-0000-0000-000000000010' WHERE tenant_id IS NULL;

UPDATE approval_tasks at
SET tenant_id = tc.tenant_id
FROM tool_calls tc
WHERE at.tool_call_id = tc.id AND at.tenant_id IS NULL;

-- 兜底:仍未回填的行归入默认租户
UPDATE approval_tasks SET tenant_id = '00000000-0000-0000-0000-000000000010' WHERE tenant_id IS NULL;

ALTER TABLE approval_tasks ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE tool_calls ALTER COLUMN tenant_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_approval_tasks_tenant ON approval_tasks(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tool_calls_tenant ON tool_calls(tenant_id, created_at DESC);
