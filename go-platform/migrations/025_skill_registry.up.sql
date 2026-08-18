-- M4-B: Skill 版本化注册与生命周期治理
-- 状态机: draft -> review -> published -> deprecated (published 起配置不可变)
-- UNIQUE (skill_code, version) 保证版本不可变
-- 附带 skill:manage 权限点 (platform_admin + business_reviewer)

CREATE TABLE IF NOT EXISTS skill_registry (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    skill_code VARCHAR(64) NOT NULL,
    version VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    config_json JSONB NOT NULL,
    created_by VARCHAR(128) NOT NULL,
    reviewed_by VARCHAR(128),
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (skill_code, version)
);

CREATE INDEX IF NOT EXISTS idx_skill_registry_code_status
    ON skill_registry (skill_code, status);

-- 权限点: skill:manage (Skill 生命周期管理面)
INSERT INTO permissions (id, code, name, resource, action) VALUES
  ('30000000-0000-0000-0000-000000000020', 'skill:manage', 'Manage Skill Lifecycle', 'skill', 'manage')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT role_id, permission_id
FROM (VALUES
  ('20000000-0000-0000-0000-000000000001'::uuid, '30000000-0000-0000-0000-000000000020'::uuid),
  ('20000000-0000-0000-0000-000000000003'::uuid, '30000000-0000-0000-0000-000000000020'::uuid)
) AS grants(role_id, permission_id)
ON CONFLICT DO NOTHING;
