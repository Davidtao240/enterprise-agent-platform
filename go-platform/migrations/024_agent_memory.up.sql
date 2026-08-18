-- M4-A: Agent 分层记忆存储 (Memory Layering)
-- 五个层级 Scope: run / thread / user / team / domain
-- - 租户强制隔离 (tenant_id)
-- - ACL 为空/[] 表示仅创建者可见 (created_by 为可见性判定依据)
-- - expires_at 过期控制 + deleted_at 软删除

CREATE TABLE IF NOT EXISTS agent_memory (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope VARCHAR(32) NOT NULL,
    scope_id VARCHAR(255) NOT NULL,
    tenant_id UUID NOT NULL,
    content JSONB NOT NULL,
    acl JSONB DEFAULT '[]',
    created_by VARCHAR(128) NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_agent_memory_scope
    ON agent_memory (scope, scope_id);
CREATE INDEX IF NOT EXISTS idx_agent_memory_tenant
    ON agent_memory (tenant_id);
CREATE INDEX IF NOT EXISTS idx_agent_memory_expires
    ON agent_memory (expires_at) WHERE expires_at IS NOT NULL;
