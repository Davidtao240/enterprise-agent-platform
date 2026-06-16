-- migrations/003_approval_and_audit.up.sql
-- Phase 3-6: Approval tasks, audit logs, agent run logs, files, business form data

CREATE TABLE approval_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_instance_id UUID NOT NULL REFERENCES workflow_instances(id),
    node_instance_id UUID NOT NULL REFERENCES workflow_node_instances(id),
    business_app_code VARCHAR(64) NOT NULL,
    title VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    assignee_role VARCHAR(64),
    assignee_user_id UUID REFERENCES users(id),
    decision_by UUID REFERENCES users(id),
    decision_comment TEXT,
    decided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id VARCHAR(128) NOT NULL,
    actor_user_id UUID REFERENCES users(id),
    business_app_code VARCHAR(64),
    action VARCHAR(128) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    resource_id VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL,
    detail_json JSONB,
    ip_address VARCHAR(64),
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_trace ON audit_logs(trace_id);
CREATE INDEX idx_audit_app ON audit_logs(business_app_code, created_at);
CREATE INDEX idx_audit_actor ON audit_logs(actor_user_id, created_at);

CREATE TABLE agent_run_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id VARCHAR(128) NOT NULL UNIQUE,
    trace_id VARCHAR(128) NOT NULL,
    workflow_instance_id UUID NOT NULL REFERENCES workflow_instances(id),
    node_instance_id UUID NOT NULL REFERENCES workflow_node_instances(id),
    business_app_code VARCHAR(64) NOT NULL,
    graph_key VARCHAR(128) NOT NULL,
    agent_id VARCHAR(128),
    status VARCHAR(32) NOT NULL,
    input_summary_json JSONB,
    output_summary_json JSONB,
    usage_json JSONB,
    error_json JSONB,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    duration_ms INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_arl_workflow ON agent_run_logs(workflow_instance_id);
CREATE INDEX idx_arl_node ON agent_run_logs(node_instance_id);

CREATE TABLE files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_instance_id UUID REFERENCES workflow_instances(id),
    business_app_code VARCHAR(64) NOT NULL,
    storage_bucket VARCHAR(128) NOT NULL,
    storage_key VARCHAR(512) NOT NULL,
    original_filename VARCHAR(255) NOT NULL,
    content_type VARCHAR(128) NOT NULL,
    size_bytes BIGINT NOT NULL,
    file_role VARCHAR(64) NOT NULL,
    uploaded_by UUID REFERENCES users(id),
    checksum VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE business_form_data (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_instance_id UUID NOT NULL REFERENCES workflow_instances(id),
    business_app_code VARCHAR(64) NOT NULL,
    form_key VARCHAR(128) NOT NULL,
    form_data JSONB NOT NULL DEFAULT '{}',
    schema_version VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
