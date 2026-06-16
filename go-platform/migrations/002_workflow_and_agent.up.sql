-- migrations/002_workflow_and_agent.up.sql
-- Phase 2-3: Workflow templates, instances, nodes, agent/tool registry, domain policies

CREATE TABLE workflow_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_app_code VARCHAR(64) NOT NULL,
    workflow_template_key VARCHAR(128) NOT NULL,
    name VARCHAR(128) NOT NULL,
    version VARCHAR(32) NOT NULL,
    graph_key VARCHAR(128) NOT NULL,
    definition_json JSONB NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (workflow_template_key, version)
);
CREATE INDEX idx_wf_templates_app ON workflow_templates(business_app_code, status);
CREATE INDEX idx_wf_templates_graph ON workflow_templates(graph_key);

CREATE TABLE workflow_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_app_code VARCHAR(64) NOT NULL,
    workflow_template_id UUID NOT NULL REFERENCES workflow_templates(id),
    workflow_template_key VARCHAR(128) NOT NULL,
    workflow_template_version VARCHAR(32) NOT NULL,
    graph_key VARCHAR(128) NOT NULL,
    title VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    input_json JSONB NOT NULL DEFAULT '{}',
    output_json JSONB,
    created_by UUID NOT NULL REFERENCES users(id),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    trace_id VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_wf_instances_app ON workflow_instances(business_app_code, status);
CREATE INDEX idx_wf_instances_user ON workflow_instances(created_by, created_at);
CREATE INDEX idx_wf_instances_trace ON workflow_instances(trace_id);

CREATE TABLE workflow_node_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_instance_id UUID NOT NULL REFERENCES workflow_instances(id),
    node_key VARCHAR(128) NOT NULL,
    node_type VARCHAR(64) NOT NULL,
    name VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    input_json JSONB,
    output_json JSONB,
    error_json JSONB,
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workflow_instance_id, node_key)
);
CREATE INDEX idx_wf_nodes_status ON workflow_node_instances(workflow_instance_id, status);

CREATE TABLE graph_registry (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    graph_key VARCHAR(128) NOT NULL UNIQUE,
    business_app_code VARCHAR(64) NOT NULL,
    name VARCHAR(128) NOT NULL,
    version VARCHAR(32) NOT NULL,
    description TEXT,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE agent_registry (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id VARCHAR(128) NOT NULL UNIQUE,
    name VARCHAR(128) NOT NULL,
    domain VARCHAR(64) NOT NULL,
    reusable_scope VARCHAR(32) NOT NULL DEFAULT 'domain_only',
    capabilities_json JSONB NOT NULL DEFAULT '[]',
    input_schema_json JSONB NOT NULL DEFAULT '{}',
    output_schema_json JSONB NOT NULL DEFAULT '{}',
    endpoint VARCHAR(255),
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE tool_registry (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tool_id VARCHAR(128) NOT NULL UNIQUE,
    name VARCHAR(128) NOT NULL,
    domain VARCHAR(64) NOT NULL,
    risk_level VARCHAR(32) NOT NULL DEFAULT 'low',
    is_shared BOOLEAN NOT NULL DEFAULT false,
    input_schema_json JSONB NOT NULL DEFAULT '{}',
    output_schema_json JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE agent_tool_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id VARCHAR(128) NOT NULL,
    tool_id VARCHAR(128) NOT NULL,
    business_app_code VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (agent_id, tool_id, business_app_code)
);

CREATE TABLE domain_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_app_code VARCHAR(64) NOT NULL UNIQUE,
    allowed_agent_domains JSONB NOT NULL DEFAULT '[]',
    allowed_tool_domains JSONB NOT NULL DEFAULT '[]',
    allow_shared_agents BOOLEAN NOT NULL DEFAULT true,
    allow_shared_tools BOOLEAN NOT NULL DEFAULT true,
    high_risk_requires_review BOOLEAN NOT NULL DEFAULT true,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
