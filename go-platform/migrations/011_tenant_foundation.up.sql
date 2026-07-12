-- V2.4: tenant identity foundation. Existing V1 data is assigned to the local default tenant.
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY,
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

INSERT INTO tenants (id, code, name, status)
VALUES ('00000000-0000-0000-0000-000000000010', 'default', 'Default Tenant', 'active')
ON CONFLICT (id) DO NOTHING;

ALTER TABLE users ADD COLUMN IF NOT EXISTS tenant_id UUID;
UPDATE users SET tenant_id = '00000000-0000-0000-0000-000000000010' WHERE tenant_id IS NULL;
ALTER TABLE users ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_tenant_id_fkey;
ALTER TABLE users ADD CONSTRAINT users_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IF NOT EXISTS idx_users_tenant ON users(tenant_id, username);

ALTER TABLE workflow_instances ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000010' REFERENCES tenants(id);
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000010' REFERENCES tenants(id);
ALTER TABLE agent_run_logs ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000010' REFERENCES tenants(id);
ALTER TABLE files ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000010' REFERENCES tenants(id);
ALTER TABLE configuration_versions ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000010' REFERENCES tenants(id);

CREATE INDEX IF NOT EXISTS idx_workflow_instances_tenant ON workflow_instances(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant ON audit_logs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_run_logs_tenant ON agent_run_logs(tenant_id, created_at DESC);
