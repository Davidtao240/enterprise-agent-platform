-- M8-A: Agent Package Dynamic Loading — versions, installations, and registration protocol

-- Ensure PostgreSQL roles exist before GRANT (idempotent, safe for existing databases)
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

-- Package versions: supports multiple versions per package, with upgrade/downgrade tracking
CREATE TABLE agent_package_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    package_code VARCHAR(64) NOT NULL,
    version VARCHAR(32) NOT NULL,
    manifest_json JSONB NOT NULL DEFAULT '{}',
    graph_key VARCHAR(128) NOT NULL,
    graph_version VARCHAR(32) DEFAULT 'v1',
    entry_type VARCHAR(32) NOT NULL DEFAULT 'conversation',
    status VARCHAR(32) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'deprecated')),
    is_current BOOLEAN NOT NULL DEFAULT false,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, package_code, version)
);

CREATE INDEX idx_package_versions_tenant ON agent_package_versions(tenant_id, package_code);
CREATE INDEX idx_package_versions_status ON agent_package_versions(tenant_id, status, is_current);

-- Package installations: per-tenant installation tracking, supports enable/disable
CREATE TABLE agent_package_installations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    package_code VARCHAR(64) NOT NULL,
    installed_version VARCHAR(32) NOT NULL,
    installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    installed_by UUID REFERENCES users(id),
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled', 'uninstalled')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, package_code)
);

CREATE INDEX idx_installations_tenant_status ON agent_package_installations(tenant_id, status);

-- Package registration protocol: tracks third-party package manifests and remote sources
CREATE TABLE agent_package_registrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    package_code VARCHAR(64) NOT NULL,
    source_type VARCHAR(32) NOT NULL
        CHECK (source_type IN ('official', 'third_party', 'marketplace')),
    source_url VARCHAR(512),
    manifest_json JSONB NOT NULL DEFAULT '{}',
    signature VARCHAR(256),
    verified BOOLEAN NOT NULL DEFAULT false,
    status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'verified', 'rejected')),
    registered_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, package_code)
);

CREATE INDEX idx_registrations_tenant ON agent_package_registrations(tenant_id, status);

-- Grant permissions for M8-A
GRANT SELECT, INSERT, UPDATE ON agent_package_versions TO "platform_admin";
GRANT SELECT, INSERT, UPDATE ON agent_package_installations TO "platform_admin";
GRANT SELECT, INSERT, UPDATE ON agent_package_registrations TO "platform_admin";

GRANT SELECT ON agent_package_versions TO "finance_manager";
GRANT SELECT ON agent_package_installations TO "finance_manager";
GRANT SELECT ON agent_package_registrations TO "finance_manager";

GRANT SELECT ON agent_package_versions TO "finance_user";
GRANT SELECT ON agent_package_installations TO "finance_user";

GRANT SELECT ON agent_package_versions TO "ops_viewer";
GRANT SELECT ON agent_package_installations TO "ops_viewer";
GRANT SELECT ON agent_package_registrations TO "ops_viewer";