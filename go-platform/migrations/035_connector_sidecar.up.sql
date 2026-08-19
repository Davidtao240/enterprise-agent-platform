-- M8-C: Connector Protocol — sidecar registration and HTTP-based connector

CREATE TABLE connector_sidecars (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    connector_code VARCHAR(64) NOT NULL,
    version VARCHAR(32) NOT NULL,
    sidecar_url VARCHAR(512) NOT NULL,
    auth_token VARCHAR(256),
    timeout_ms INTEGER NOT NULL DEFAULT 30000,
    capabilities_json JSONB NOT NULL DEFAULT '[]',
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'inactive', 'error')),
    health_status VARCHAR(32) NOT NULL DEFAULT 'unknown'
        CHECK (health_status IN ('unknown', 'healthy', 'unhealthy')),
    last_health_check TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, connector_code, version)
);

CREATE INDEX idx_sidecars_tenant ON connector_sidecars(tenant_id, status);
CREATE INDEX idx_sidecars_code ON connector_sidecars(connector_code, version, status);

-- Grant permissions
GRANT SELECT, INSERT, UPDATE ON connector_sidecars TO "platform_admin";
GRANT SELECT ON connector_sidecars TO "finance_manager";
GRANT SELECT ON connector_sidecars TO "ops_viewer";