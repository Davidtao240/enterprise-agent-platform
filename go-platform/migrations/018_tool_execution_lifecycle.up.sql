-- M2-C: Tool Execution Lifecycle
-- 1) approval_tasks 支持 Tool Call 审批:workflow/node 可空 + tool_call_id + payload_hash
--    (payload_hash 绑定 tool_calls.input_hash,保证审批 Payload 与执行 Payload 一致)
-- 2) tool_calls 生命周期字段:timeout_at / retry_count
-- 3) 状态机由应用层守卫:requested/pending_approval/executing → 终态

-- approval_tasks:Tool Call 审批可能不关联 Workflow(纯 Agent Run 内工具调用)
ALTER TABLE approval_tasks ALTER COLUMN workflow_instance_id DROP NOT NULL;
ALTER TABLE approval_tasks ALTER COLUMN node_instance_id DROP NOT NULL;

ALTER TABLE approval_tasks
ADD COLUMN IF NOT EXISTS tool_call_id UUID REFERENCES tool_calls(id);

-- 审批绑定的不可变 Payload 哈希(= tool_calls.input_hash)
ALTER TABLE approval_tasks
ADD COLUMN IF NOT EXISTS payload_hash VARCHAR(128);

-- 一个 Tool Call 至多一条审批任务(幂等创建依赖此约束)
CREATE UNIQUE INDEX IF NOT EXISTS uq_approval_tasks_tool_call
ON approval_tasks (tool_call_id)
WHERE tool_call_id IS NOT NULL;

-- tool_calls:业务域(生命周期自包含:审批/重检不再依赖调用方回传)、超时与重试计数
ALTER TABLE tool_calls ADD COLUMN IF NOT EXISTS business_app_code VARCHAR(64) NOT NULL DEFAULT 'platform';
ALTER TABLE tool_calls ADD COLUMN IF NOT EXISTS timeout_at TIMESTAMPTZ;
ALTER TABLE tool_calls ADD COLUMN IF NOT EXISTS retry_count INT NOT NULL DEFAULT 0;
