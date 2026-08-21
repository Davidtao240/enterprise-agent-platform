-- 039_fix_migration_chain.up.sql
-- 修复迁移链中断问题：Migration 033 因 PostgreSQL 角色不存在而失败

-- 步骤 1: 创建必要的 PostgreSQL 角色（如果不存在）
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'platform_admin') THEN
        CREATE ROLE platform_admin;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'finance_manager') THEN
        CREATE ROLE finance_manager;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'finance_user') THEN
        CREATE ROLE finance_user;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ops_viewer') THEN
        CREATE ROLE ops_viewer;
    END IF;
END
$$;

-- 步骤 2: 重新执行 Migration 033 的核心表结构（跳过 GRANT，因为角色已在上面创建）
-- Agent Package Dynamic Loading

CREATE TABLE IF NOT EXISTS agent_package_registrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_code VARCHAR(128) NOT NULL UNIQUE,
    package_name VARCHAR(255) NOT NULL,
    description TEXT,
    version VARCHAR(32) NOT NULL DEFAULT '1.0.0',
    author VARCHAR(255),
    business_app_code VARCHAR(64),
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    registration_type VARCHAR(32) NOT NULL DEFAULT 'external',
    config_schema JSONB DEFAULT '{}',
    metadata_json JSONB DEFAULT '{}',
    created_by UUID,
    tenant_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_agent_package_registrations_status ON agent_package_registrations(status);
CREATE INDEX IF NOT EXISTS idx_agent_package_registrations_code ON agent_package_registrations(package_code);
CREATE INDEX IF NOT EXISTS idx_agent_package_registrations_tenant ON agent_package_registrations(tenant_id);

CREATE TABLE IF NOT EXISTS agent_package_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    registration_id UUID NOT NULL REFERENCES agent_package_registrations(id) ON DELETE CASCADE,
    version VARCHAR(32) NOT NULL,
    entry_point VARCHAR(512),
    dependencies JSONB DEFAULT '[]',
    capabilities JSONB DEFAULT '[]',
    config_schema JSONB DEFAULT '{}',
    changelog TEXT,
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(registration_id, version)
);

CREATE INDEX IF NOT EXISTS idx_agent_package_versions_status ON agent_package_versions(status);
CREATE INDEX IF NOT EXISTS idx_agent_package_versions_registration ON agent_package_versions(registration_id);

CREATE TABLE IF NOT EXISTS agent_package_installations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_code VARCHAR(128) NOT NULL,
    version_id UUID NOT NULL REFERENCES agent_package_versions(id) ON DELETE CASCADE,
    installed_by UUID,
    tenant_id UUID NOT NULL,
    business_app_code VARCHAR(64),
    installation_path VARCHAR(512),
    config JSONB DEFAULT '{}',
    status VARCHAR(32) NOT NULL DEFAULT 'installed',
    installed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(package_code, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_package_installations_tenant ON agent_package_installations(tenant_id);
CREATE INDEX IF NOT EXISTS idx_agent_package_installations_status ON agent_package_installations(status);

-- 步骤 3: 重新执行 Migration 034 - Skill Marketplace
CREATE TABLE IF NOT EXISTS skill_installations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    skill_code VARCHAR(128) NOT NULL,
    skill_version VARCHAR(32),
    installed_by UUID,
    tenant_id UUID NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'installed',
    installed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(skill_code, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_skill_installations_tenant ON skill_installations(tenant_id);

CREATE TABLE IF NOT EXISTS skill_usage_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    skill_code VARCHAR(128) NOT NULL,
    version VARCHAR(32),
    user_id UUID,
    tenant_id UUID NOT NULL,
    event_type VARCHAR(32) NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_skill_usage_events_skill ON skill_usage_events(skill_code);
CREATE INDEX IF NOT EXISTS idx_skill_usage_events_tenant ON skill_usage_events(tenant_id);
CREATE INDEX IF NOT EXISTS idx_skill_usage_events_time ON skill_usage_events(created_at DESC);

-- 步骤 4: 重新执行 Migration 035 - Connector Sidecar
CREATE TABLE IF NOT EXISTS sidecar_registrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(128) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    connector_type VARCHAR(32) NOT NULL,
    http_method VARCHAR(16) NOT NULL DEFAULT 'POST',
    endpoint_url VARCHAR(512) NOT NULL,
    auth_type VARCHAR(32) NOT NULL DEFAULT 'none',
    auth_config JSONB DEFAULT '{}',
    request_template JSONB DEFAULT '{}',
    response_mapping JSONB DEFAULT '{}',
    timeout_ms INT NOT NULL DEFAULT 30000,
    retry_policy JSONB DEFAULT '{}',
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    tenant_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sidecar_registrations_status ON sidecar_registrations(status);
CREATE INDEX IF NOT EXISTS idx_sidecar_registrations_tenant ON sidecar_registrations(tenant_id);

-- 步骤 5: 重新执行 Migration 036 - Knowledge Base
CREATE TABLE IF NOT EXISTS knowledge_collections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    metadata_json JSONB NOT NULL DEFAULT '{}',
    created_by UUID,
    updated_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_knowledge_collections_tenant ON knowledge_collections(tenant_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_collections_name ON knowledge_collections(name);

-- 确保 knowledge_documents 表存在
CREATE TABLE IF NOT EXISTS knowledge_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL,
    filename VARCHAR(512) NOT NULL,
    file_type VARCHAR(32),
    file_size BIGINT,
    storage_key VARCHAR(512),
    metadata_json JSONB DEFAULT '{}',
    uploaded_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_knowledge_documents_collection ON knowledge_documents(collection_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_documents_tenant ON knowledge_documents(tenant_id);

-- 确保 knowledge_chunks 表存在
CREATE TABLE IF NOT EXISTS knowledge_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
    chunk_index INT NOT NULL,
    content TEXT NOT NULL,
    embedding vector(1536),
    metadata_json JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_document ON knowledge_chunks(document_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_index ON knowledge_chunks(chunk_index);

-- 步骤 6: 为 agent_run_logs 添加 estimated_cost 列（如果不存在）
ALTER TABLE agent_run_logs ADD COLUMN IF NOT EXISTS estimated_cost NUMERIC(12,4) DEFAULT 0;
ALTER TABLE agent_run_logs ADD COLUMN IF NOT EXISTS duration_ms INT DEFAULT 0;

-- 步骤 7: 执行 Migration 038 的权限修复
-- 为 ops_viewer 添加缺失的 :read 权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000004', p.id
FROM permissions p
WHERE p.code = 'agent:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000004' 
    AND rp.permission_id = p.id
);

INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000004', p.id
FROM permissions p
WHERE p.code = 'tool:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000004' 
    AND rp.permission_id = p.id
);

INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000004', p.id
FROM permissions p
WHERE p.code = 'trace:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000004' 
    AND rp.permission_id = p.id
);

INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000004', p.id
FROM permissions p
WHERE p.code = 'observability:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000004' 
    AND rp.permission_id = p.id
);

INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000004', p.id
FROM permissions p
WHERE p.code = 'outbox:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000004' 
    AND rp.permission_id = p.id
);

-- 为 finance_user 添加对话和查看权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000002', p.id
FROM permissions p
WHERE p.code IN ('conversation:read', 'conversation:write', 'agent:read', 'skill:read')
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000002' 
    AND rp.permission_id = p.id
);

-- 为 finance_manager 添加对话和 skill:read 权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000003', p.id
FROM permissions p
WHERE p.code IN ('conversation:read', 'conversation:write', 'skill:read')
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000003' 
    AND rp.permission_id = p.id
);

-- 添加缺失的权限定义（如果不存在）
-- 注意：ID 已分配唯一值，001-025 为早期迁移预留，此处从 101 开始避免主键冲突
INSERT INTO permissions (id, code, name, resource, action) VALUES
('30000000-0000-0000-0000-000000000101', 'agent:read', 'Read Agents', 'agent', 'read'),
('30000000-0000-0000-0000-000000000102', 'tool:read', 'Read Tools', 'tool', 'read'),
('30000000-0000-0000-0000-000000000103', 'skill:read', 'Read Skills', 'skill', 'read'),
('30000000-0000-0000-0000-000000000104', 'trace:read', 'Read Traces', 'trace', 'read'),
('30000000-0000-0000-0000-000000000105', 'observability:read', 'Read Observability', 'observability', 'read'),
('30000000-0000-0000-0000-000000000106', 'outbox:read', 'Read Outbox', 'outbox', 'read'),
('30000000-0000-0000-0000-000000000107', 'conversation:read', 'Read Conversations', 'conversation', 'read'),
('30000000-0000-0000-0000-000000000108', 'conversation:write', 'Write Conversations', 'conversation', 'write')
ON CONFLICT (code) DO NOTHING;
