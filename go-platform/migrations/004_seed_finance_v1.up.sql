-- migrations/004_seed_finance_v1.up.sql
-- V1 seed data: finance business app, workflow template, graph, agents, tools, domain policy, demo users

-- Departments
INSERT INTO departments (id, code, name, status) VALUES
  ('d0000000-0000-0000-0000-000000000001', 'platform', 'Platform Team', 'active'),
  ('d0000000-0000-0000-0000-000000000002', 'finance', 'Finance Center', 'active'),
  ('d0000000-0000-0000-0000-000000000003', 'ops', 'Operations Team', 'active');

-- Users (password is "password" — bcrypt hash)
INSERT INTO users (id, username, display_name, email, password_hash, department_id, status) VALUES
  ('u0000000-0000-0000-0000-000000000001', 'admin', 'Platform Admin', 'admin@example.com', '$2b$12$fAwdBkh1ib79/S4HRAOGZe99jA7b/uaJXlf/t9MSM4iUIMXVQvgdS', 'd0000000-0000-0000-0000-000000000001', 'active'),
  ('u0000000-0000-0000-0000-000000000002', 'finance_user', 'Finance User', 'finance_user@example.com', '$2b$12$fAwdBkh1ib79/S4HRAOGZe99jA7b/uaJXlf/t9MSM4iUIMXVQvgdS', 'd0000000-0000-0000-0000-000000000002', 'active'),
  ('u0000000-0000-0000-0000-000000000003', 'finance_manager', 'Finance Manager', 'finance_manager@example.com', '$2b$12$fAwdBkh1ib79/S4HRAOGZe99jA7b/uaJXlf/t9MSM4iUIMXVQvgdS', 'd0000000-0000-0000-0000-000000000002', 'active'),
  ('u0000000-0000-0000-0000-000000000004', 'ops_viewer', 'Ops Viewer', 'ops@example.com', '$2b$12$fAwdBkh1ib79/S4HRAOGZe99jA7b/uaJXlf/t9MSM4iUIMXVQvgdS', 'd0000000-0000-0000-0000-000000000003', 'active');

-- Roles
INSERT INTO roles (id, code, name) VALUES
  ('r0000000-0000-0000-0000-000000000001', 'platform_admin', 'Platform Admin'),
  ('r0000000-0000-0000-0000-000000000002', 'business_user', 'Business User'),
  ('r0000000-0000-0000-0000-000000000003', 'business_reviewer', 'Business Reviewer'),
  ('r0000000-0000-0000-0000-000000000004', 'ops_viewer', 'Ops Viewer');

-- Permissions
INSERT INTO permissions (id, code, name, resource, action) VALUES
  ('p0000000-0000-0000-0000-000000000001', 'business_app:read', 'Read Business Apps', 'business_app', 'read'),
  ('p0000000-0000-0000-0000-000000000002', 'workflow_template:read', 'Read Workflow Templates', 'workflow_template', 'read'),
  ('p0000000-0000-0000-0000-000000000003', 'workflow:create', 'Create Workflow', 'workflow', 'create'),
  ('p0000000-0000-0000-0000-000000000004', 'workflow:read', 'Read Workflow', 'workflow', 'read'),
  ('p0000000-0000-0000-0000-000000000005', 'workflow:start', 'Start Workflow', 'workflow', 'start'),
  ('p0000000-0000-0000-0000-000000000006', 'workflow:cancel', 'Cancel Workflow', 'workflow', 'cancel'),
  ('p0000000-0000-0000-0000-000000000007', 'workflow:retry', 'Retry Workflow', 'workflow', 'retry'),
  ('p0000000-0000-0000-0000-000000000008', 'file:upload', 'Upload File', 'file', 'upload'),
  ('p0000000-0000-0000-0000-000000000009', 'file:read', 'Read File', 'file', 'read'),
  ('p0000000-0000-0000-0000-000000000010', 'approval:read', 'Read Approvals', 'approval', 'read'),
  ('p0000000-0000-0000-0000-000000000011', 'approval:decide', 'Decide Approvals', 'approval', 'decide'),
  ('p0000000-0000-0000-0000-000000000012', 'audit:read', 'Read Audit Logs', 'audit', 'read'),
  ('p0000000-0000-0000-0000-000000000013', 'agent:manage', 'Manage Agents', 'agent', 'manage'),
  ('p0000000-0000-0000-0000-000000000014', 'tool:manage', 'Manage Tools', 'tool', 'manage'),
  ('p0000000-0000-0000-0000-000000000015', 'user:manage', 'Manage Users', 'user', 'manage'),
  ('p0000000-0000-0000-0000-000000000016', 'role:manage', 'Manage Roles', 'role', 'manage');

-- Business App
INSERT INTO business_apps (id, code, name, description, icon, sort_order, status) VALUES
  ('b0000000-0000-0000-0000-000000000001', 'finance', 'Finance Center', 'Operating data reporting, finance analysis, report review, and archive.', 'chart', 10, 'active');

-- Graph Registry
INSERT INTO graph_registry (id, graph_key, business_app_code, name, version, description, status) VALUES
  ('g0000000-0000-0000-0000-000000000001', 'finance_operating_report_graph', 'finance', 'Finance Operating Report Graph', '1.0.0', 'Extract, validate, analyze, and generate finance operating report.', 'active');

-- Workflow Template
INSERT INTO workflow_templates (id, business_app_code, workflow_template_key, name, version, graph_key, definition_json, status) VALUES
  ('w0000000-0000-0000-0000-000000000001', 'finance', 'finance_operating_report', 'Operating Data Report', '1.0.0', 'finance_operating_report_graph',
   '{"nodes":[{"id":"upload","type":"file_upload","name":"Upload Operating Data","required":true},{"id":"agent_graph","type":"agent_graph","name":"AI Finance Analysis","graph_key":"finance_operating_report_graph"},{"id":"human_review","type":"human_review","name":"Finance Manager Review","role":"business_reviewer","required":true},{"id":"archive","type":"system","name":"Archive Report","action":"archive_result"}],"edges":[{"from":"upload","to":"agent_graph"},{"from":"agent_graph","to":"human_review","when":"succeeded"},{"from":"human_review","to":"archive","when":"approved"}]}',
   'active');

-- Agent Registry
INSERT INTO agent_registry (id, agent_id, name, domain, reusable_scope, capabilities_json, status) VALUES
  ('a0000000-0000-0000-0000-000000000001', 'data_extract_agent', 'Data Extract Agent', 'shared', 'shared', '["extract_table","parse_csv","parse_excel"]', 'active'),
  ('a0000000-0000-0000-0000-000000000002', 'schema_mapping_agent', 'Schema Mapping Agent', 'shared', 'shared', '["normalize_fields","map_schema"]', 'active'),
  ('a0000000-0000-0000-0000-000000000003', 'validation_agent', 'Validation Agent', 'shared', 'shared', '["validate_required_fields","detect_outliers"]', 'active'),
  ('a0000000-0000-0000-0000-000000000004', 'finance_analysis_agent', 'Finance Analysis Agent', 'finance', 'domain_only', '["metric_analysis","trend_summary","risk_explanation"]', 'active'),
  ('a0000000-0000-0000-0000-000000000005', 'report_agent', 'Report Agent', 'shared', 'shared', '["report_generation","summary_generation"]', 'active'),
  ('a0000000-0000-0000-0000-000000000006', 'review_summary_agent', 'Review Summary Agent', 'shared', 'shared', '["review_summary","warning_summary"]', 'active');

-- Tool Registry
INSERT INTO tool_registry (id, tool_id, name, domain, risk_level, is_shared, status) VALUES
  ('t0000000-0000-0000-0000-000000000001', 'parse_csv', 'Parse CSV', 'shared', 'low', true, 'active'),
  ('t0000000-0000-0000-0000-000000000002', 'parse_excel', 'Parse Excel', 'shared', 'low', true, 'active'),
  ('t0000000-0000-0000-0000-000000000003', 'normalize_finance_schema', 'Normalize Finance Schema', 'finance', 'low', false, 'active'),
  ('t0000000-0000-0000-0000-000000000004', 'validate_finance_metrics', 'Validate Finance Metrics', 'finance', 'medium', false, 'active'),
  ('t0000000-0000-0000-0000-000000000005', 'generate_finance_report', 'Generate Finance Report', 'finance', 'medium', false, 'active'),
  ('t0000000-0000-0000-0000-000000000006', 'archive_report', 'Archive Report', 'finance', 'high', false, 'active');

-- Agent Tool Permissions
INSERT INTO agent_tool_permissions (id, agent_id, tool_id, business_app_code, status) VALUES
  ('atp00000-0000-0000-0000-000000000001', 'data_extract_agent', 'parse_csv', 'finance', 'active'),
  ('atp00000-0000-0000-0000-000000000002', 'data_extract_agent', 'parse_excel', 'finance', 'active'),
  ('atp00000-0000-0000-0000-000000000003', 'schema_mapping_agent', 'normalize_finance_schema', 'finance', 'active'),
  ('atp00000-0000-0000-0000-000000000004', 'validation_agent', 'validate_finance_metrics', 'finance', 'active'),
  ('atp00000-0000-0000-0000-000000000005', 'finance_analysis_agent', 'validate_finance_metrics', 'finance', 'active'),
  ('atp00000-0000-0000-0000-000000000006', 'report_agent', 'generate_finance_report', 'finance', 'active'),
  ('atp00000-0000-0000-0000-000000000007', 'review_summary_agent', 'generate_finance_report', 'finance', 'active');

-- User-Role assignments
INSERT INTO user_roles (user_id, role_id) VALUES
  ('u0000000-0000-0000-0000-000000000001', 'r0000000-0000-0000-0000-000000000001'),
  ('u0000000-0000-0000-0000-000000000002', 'r0000000-0000-0000-0000-000000000002'),
  ('u0000000-0000-0000-0000-000000000003', 'r0000000-0000-0000-0000-000000000003'),
  ('u0000000-0000-0000-0000-000000000004', 'r0000000-0000-0000-0000-000000000004');

-- Role-Permission mappings
-- platform_admin: all permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'r0000000-0000-0000-0000-000000000001', id FROM permissions;

-- business_user
INSERT INTO role_permissions (role_id, permission_id) VALUES
  ('r0000000-0000-0000-0000-000000000002', 'p0000000-0000-0000-0000-000000000001'),
  ('r0000000-0000-0000-0000-000000000002', 'p0000000-0000-0000-0000-000000000002'),
  ('r0000000-0000-0000-0000-000000000002', 'p0000000-0000-0000-0000-000000000003'),
  ('r0000000-0000-0000-0000-000000000002', 'p0000000-0000-0000-0000-000000000004'),
  ('r0000000-0000-0000-0000-000000000002', 'p0000000-0000-0000-0000-000000000005'),
  ('r0000000-0000-0000-0000-000000000002', 'p0000000-0000-0000-0000-000000000008'),
  ('r0000000-0000-0000-0000-000000000002', 'p0000000-0000-0000-0000-000000000009');

-- business_reviewer
INSERT INTO role_permissions (role_id, permission_id) VALUES
  ('r0000000-0000-0000-0000-000000000003', 'p0000000-0000-0000-0000-000000000001'),
  ('r0000000-0000-0000-0000-000000000003', 'p0000000-0000-0000-0000-000000000002'),
  ('r0000000-0000-0000-0000-000000000003', 'p0000000-0000-0000-0000-000000000004'),
  ('r0000000-0000-0000-0000-000000000003', 'p0000000-0000-0000-0000-000000000009'),
  ('r0000000-0000-0000-0000-000000000003', 'p0000000-0000-0000-0000-000000000010'),
  ('r0000000-0000-0000-0000-000000000003', 'p0000000-0000-0000-0000-000000000011');

-- ops_viewer
INSERT INTO role_permissions (role_id, permission_id) VALUES
  ('r0000000-0000-0000-0000-000000000004', 'p0000000-0000-0000-0000-000000000001'),
  ('r0000000-0000-0000-0000-000000000004', 'p0000000-0000-0000-0000-000000000002'),
  ('r0000000-0000-0000-0000-000000000004', 'p0000000-0000-0000-0000-000000000004'),
  ('r0000000-0000-0000-0000-000000000004', 'p0000000-0000-0000-0000-000000000007'),
  ('r0000000-0000-0000-0000-000000000004', 'p0000000-0000-0000-0000-000000000009'),
  ('r0000000-0000-0000-0000-000000000004', 'p0000000-0000-0000-0000-000000000010'),
  ('r0000000-0000-0000-0000-000000000004', 'p0000000-0000-0000-0000-000000000012');

-- Domain Policy
INSERT INTO domain_policies (id, business_app_code, allowed_agent_domains, allowed_tool_domains, allow_shared_agents, allow_shared_tools, high_risk_requires_review, status) VALUES
  ('dp000000-0000-0000-0000-000000000001', 'finance', '["finance","shared"]', '["finance","shared"]', true, true, true, 'active');
