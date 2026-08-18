-- M3-A: Connector Registry 注册与版本治理 + Binding 发布阶段门禁列
-- 领域中立:注册表只描述 Connector 契约(能力/认证/阶段),不含业务逻辑。

-- Connector 注册表:版本不可变,(connector_code, version) 唯一。
-- release_stage 是发布阶段门禁的机器可执行依据:
--   mock_fixture → sandbox_readonly → shadow → human_approved_write → limited_canary → production
CREATE TABLE IF NOT EXISTS connector_registry (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connector_code VARCHAR(128) NOT NULL,          -- 逻辑标识(如 enterprise_db_read_connector)
    version VARCHAR(32) NOT NULL,                  -- 语义化版本,不可变
    connector_type VARCHAR(64) NOT NULL,           -- db_read / ticket / erp / mock
    capabilities_json JSONB NOT NULL DEFAULT '[]', -- 能力清单(name + input/output schema)
    auth_type VARCHAR(32) NOT NULL DEFAULT 'none', -- none / api_key / oauth2 / basic
    health_check_json JSONB,                       -- 健康检查契约配置
    release_stage VARCHAR(32) NOT NULL DEFAULT 'mock_fixture',
    status VARCHAR(16) NOT NULL DEFAULT 'draft',   -- draft / active / deprecated
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_connector_registry_code_version UNIQUE (connector_code, version),
    CONSTRAINT ck_connector_registry_stage CHECK (release_stage IN
        ('mock_fixture','sandbox_readonly','shadow','human_approved_write','limited_canary','production')),
    CONSTRAINT ck_connector_registry_status CHECK (status IN ('draft','active','deprecated')),
    CONSTRAINT ck_connector_registry_type CHECK (connector_type IN ('db_read','ticket','erp','mock')),
    CONSTRAINT ck_connector_registry_auth CHECK (auth_type IN ('none','api_key','oauth2','basic'))
);
CREATE INDEX IF NOT EXISTS idx_connector_registry_code
ON connector_registry (connector_code, status);

-- Binding 补列:发布阶段门禁 + capability 白名单 + 不可变版本绑定
ALTER TABLE connector_bindings
ADD COLUMN IF NOT EXISTS environment VARCHAR(16) NOT NULL DEFAULT 'mock',
ADD COLUMN IF NOT EXISTS allowed_capabilities JSONB NOT NULL DEFAULT '[]',
ADD COLUMN IF NOT EXISTS connector_version VARCHAR(32);
ALTER TABLE connector_bindings
DROP CONSTRAINT IF EXISTS ck_connector_binding_env;
ALTER TABLE connector_bindings
ADD CONSTRAINT ck_connector_binding_env
CHECK (environment IN ('mock','sandbox','shadow','production'));
CREATE INDEX IF NOT EXISTS idx_connector_bindings_env
ON connector_bindings (tenant_id, environment);

-- 注册首批 Connector: Mock db_read(M3-A 演示基线,只读)
INSERT INTO connector_registry
    (connector_code, version, connector_type, capabilities_json, auth_type, release_stage, status)
VALUES
    ('enterprise_db_read_connector', '1.0.0', 'mock',
     '[{"name":"enterprise_db_read","kind":"read","input_schema":{"query_template":"string","params":"object"},"output_schema":{"rows":"array","row_count":"integer"}}]'::jsonb,
     'none', 'mock_fixture', 'active')
ON CONFLICT (connector_code, version) DO NOTHING;
