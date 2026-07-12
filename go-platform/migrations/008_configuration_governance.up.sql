-- V2.1: domain-neutral configuration governance.
CREATE TABLE configuration_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type VARCHAR(64) NOT NULL,
    resource_key VARCHAR(128) NOT NULL,
    version VARCHAR(32) NOT NULL,
    lifecycle_status VARCHAR(32) NOT NULL DEFAULT 'draft',
    snapshot_json JSONB NOT NULL DEFAULT '{}',
    change_summary TEXT NOT NULL DEFAULT '',
    created_by UUID NOT NULL REFERENCES users(id),
    approved_by UUID REFERENCES users(id),
    approved_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    deprecated_at TIMESTAMPTZ,
    trace_id VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (resource_type, resource_key, version),
    CHECK (resource_type IN ('business_app', 'workflow_template', 'agent', 'tool', 'domain_policy')),
    CHECK (lifecycle_status IN ('draft', 'pending_approval', 'published', 'deprecated'))
);
CREATE INDEX idx_configuration_versions_resource ON configuration_versions(resource_type, resource_key, created_at DESC);
CREATE INDEX idx_configuration_versions_status ON configuration_versions(lifecycle_status, created_at DESC);

INSERT INTO permissions (id, code, name, resource, action) VALUES
  ('30000000-0000-0000-0000-000000000017', 'configuration:manage', 'Manage Configuration Changes', 'configuration', 'manage'),
  ('30000000-0000-0000-0000-000000000018', 'configuration:approve', 'Approve Configuration Changes', 'configuration', 'approve')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000001', id
FROM permissions
WHERE code IN ('configuration:manage', 'configuration:approve')
ON CONFLICT DO NOTHING;
