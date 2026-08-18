-- M2-D: Connector Binding + CredentialRef 边界
-- 原则:Secret 明文只存在于加密列(cipher_text)与执行瞬间的内存,
-- 不落日志/审计/tool_calls,不进模型上下文。

-- Connector 绑定:工具的外部系统接入配置(不含明文凭证)
CREATE TABLE IF NOT EXISTS connector_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    business_app_code VARCHAR(64) NOT NULL,
    connector_code VARCHAR(128) NOT NULL,          -- 外部系统标识(如 erp_sap)
    name VARCHAR(256) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active',  -- active / disabled
    config_json JSONB NOT NULL DEFAULT '{}',       -- 非敏感配置(endpoint/timeout 等)
    credential_ref VARCHAR(128),                   -- 'secret:<uuid>' 指向 credential_secrets
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_connector_binding_status CHECK (status IN ('active','disabled'))
);
CREATE INDEX IF NOT EXISTS idx_connector_bindings_tenant
ON connector_bindings (tenant_id, business_app_code);

-- 凭证密文存储:AES-256-GCM,密钥来自环境变量,绝不入库
CREATE TABLE IF NOT EXISTS credential_secrets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cipher_text TEXT NOT NULL,                     -- base64(nonce+ ciphertext+tag)
    key_hint VARCHAR(64) NOT NULL DEFAULT 'v1',    -- 密钥版本提示(轮换用,非密钥本身)
    algorithm VARCHAR(32) NOT NULL DEFAULT 'AES-256-GCM',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
