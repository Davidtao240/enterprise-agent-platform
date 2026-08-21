-- M8-B: Skill Marketplace — metadata, categories, installations, usage tracking

ALTER TABLE skill_registry
    ADD COLUMN IF NOT EXISTS name VARCHAR(256) DEFAULT '',
    ADD COLUMN IF NOT EXISTS description TEXT DEFAULT '',
    ADD COLUMN IF NOT EXISTS category VARCHAR(64) DEFAULT 'general'
        CHECK (category IN ('general', 'finance', 'hr', 'procurement', 'legal', 'it', 'customer_service', 'productivity')),
    ADD COLUMN IF NOT EXISTS icon VARCHAR(64) DEFAULT '',
    ADD COLUMN IF NOT EXISTS tags JSONB DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS author VARCHAR(128) DEFAULT '',
    ADD COLUMN IF NOT EXISTS homepage_url VARCHAR(512) DEFAULT '',
    ADD COLUMN IF NOT EXISTS is_current BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS usage_count INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_skill_registry_category ON skill_registry(category, status);
CREATE INDEX IF NOT EXISTS idx_skill_registry_code_status ON skill_registry(skill_code, status, is_current);

-- Skill installations: per-user/tenant installation tracking
CREATE TABLE skill_installations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    user_id UUID NOT NULL REFERENCES users(id),
    skill_id UUID NOT NULL REFERENCES skill_registry(id) ON DELETE CASCADE,
    skill_code VARCHAR(64) NOT NULL,
    installed_version VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled', 'uninstalled')),
    installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, user_id, skill_code)
);

CREATE INDEX idx_skill_installations_user ON skill_installations(tenant_id, user_id, status);
CREATE INDEX idx_skill_installations_skill ON skill_installations(tenant_id, skill_code, status);

-- Skill usage events: for tracking when skills are invoked
CREATE TABLE skill_usage_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    skill_code VARCHAR(64) NOT NULL,
    skill_version VARCHAR(32),
    user_id UUID REFERENCES users(id),
    session_id VARCHAR(128),
    tool_call_id VARCHAR(128),
    used_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_skill_usage_events_skill ON skill_usage_events(tenant_id, skill_code, used_at DESC);
CREATE INDEX idx_skill_usage_events_user ON skill_usage_events(tenant_id, user_id, used_at DESC);

-- Grant permissions
GRANT SELECT, INSERT, UPDATE ON skill_installations TO "platform_admin";
GRANT SELECT, INSERT, UPDATE ON skill_installations TO "finance_manager";
GRANT SELECT, INSERT ON skill_installations TO "finance_user";
GRANT SELECT ON skill_installations TO "ops_viewer";

GRANT SELECT, INSERT ON skill_usage_events TO "platform_admin";
GRANT SELECT, INSERT ON skill_usage_events TO "finance_manager";
GRANT SELECT ON skill_usage_events TO "finance_user";
GRANT SELECT ON skill_usage_events TO "ops_viewer";

-- M8-B: Add skill:read permission for marketplace access
INSERT INTO permissions (id, code, name, resource, action)
VALUES ('30000000-0000-0000-0000-000000000026', 'skill:read', 'Read Skill Marketplace', 'skill', 'read')
ON CONFLICT (code) DO NOTHING;