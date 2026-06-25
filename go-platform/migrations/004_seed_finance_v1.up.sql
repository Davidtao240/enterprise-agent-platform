-- migrations/004_seed_finance_v1.up.sql
-- V1 seed data: finance business app, workflow template, graph, agents, tools, domain policy, demo users

-- Departments
INSERT INTO departments (id, code, name, status) VALUES
  ('d0000000-0000-0000-0000-000000000001', 'platform', 'Platform Team', 'active'),
  ('d0000000-0000-0000-0000-000000000002', 'finance', 'Finance Center', 'active'),
  ('d0000000-0000-0000-0000-000000000003', 'ops', 'Operations Team', 'active');

-- Users (password is "password" — bcrypt hash)
INSERT INTO users (id, username, display_name, email, password_hash, department_id, status) VALUES
  ('10000000-0000-0000-0000-000000000001', 'admin', 'Platform Admin', 'admin@example.com', '$2b$12$fAwdBkh1ib79/S4HRAOGZe99jA7b/uaJXlf/t9MSM4iUIMXVQvgdS', 'd0000000-0000-0000-0000-000000000001', 'active'),
  ('10000000-0000-0000-0000-000000000002', 'finance_user', 'Finance User', 'finance_user@example.com', '$2b$12$fAwdBkh1ib79/S4HRAOGZe99jA7b/uaJXlf/t9MSM4iUIMXVQvgdS', 'd0000000-0000-0000-0000-000000000002', 'active'),
  ('10000000-0000-0000-0000-000000000003', 'finance_manager', 'Finance Manager', 'finance_manager@example.com', '$2b$12$fAwdBkh1ib79/S4HRAOGZe99jA7b/uaJXlf/t9MSM4iUIMXVQvgdS', 'd0000000-0000-0000-0000-000000000002', 'active'),
  ('10000000-0000-0000-0000-000000000004', 'ops_viewer', 'Ops Viewer', 'ops@example.com', '$2b$12$fAwdBkh1ib79/S4HRAOGZe99jA7b/uaJXlf/t9MSM4iUIMXVQvgdS', 'd0000000-0000-0000-0000-000000000003', 'active');

-- Roles
INSERT INTO roles (id, code, name) VALUES
  ('20000000-0000-0000-0000-000000000001', 'platform_admin', 'Platform Admin'),
  ('20000000-0000-0000-0000-000000000002', 'business_user', 'Business User'),
  ('20000000-0000-0000-0000-000000000003', 'business_reviewer', 'Business Reviewer'),
  ('20000000-0000-0000-0000-000000000004', 'ops_viewer', 'Ops Viewer');

-- Permissions
INSERT INTO permissions (id, code, name, resource, action) VALUES
  ('30000000-0000-0000-0000-000000000001', 'business_app:read', 'Read Business Apps', 'business_app', 'read'),
  ('30000000-0000-0000-0000-000000000002', 'workflow_template:read', 'Read Workflow Templates', 'workflow_template', 'read'),
  ('30000000-0000-0000-0000-000000000003', 'workflow:create', 'Create Workflow', 'workflow', 'create'),
  ('30000000-0000-0000-0000-000000000004', 'workflow:read', 'Read Workflow', 'workflow', 'read'),
  ('30000000-0000-0000-0000-000000000005', 'workflow:start', 'Start Workflow', 'workflow', 'start'),
  ('30000000-0000-0000-0000-000000000006', 'workflow:cancel', 'Cancel Workflow', 'workflow', 'cancel'),
  ('30000000-0000-0000-0000-000000000007', 'workflow:retry', 'Retry Workflow', 'workflow', 'retry'),
  ('30000000-0000-0000-0000-000000000008', 'file:upload', 'Upload File', 'file', 'upload'),
  ('30000000-0000-0000-0000-000000000009', 'file:read', 'Read File', 'file', 'read'),
  ('30000000-0000-0000-0000-000000000010', 'approval:read', 'Read Approvals', 'approval', 'read'),
  ('30000000-0000-0000-0000-000000000011', 'approval:decide', 'Decide Approvals', 'approval', 'decide'),
  ('30000000-0000-0000-0000-000000000012', 'audit:read', 'Read Audit Logs', 'audit', 'read'),
  ('30000000-0000-0000-0000-000000000013', 'agent:manage', 'Manage Agents', 'agent', 'manage'),
  ('30000000-0000-0000-0000-000000000014', 'tool:manage', 'Manage Tools', 'tool', 'manage'),
  ('30000000-0000-0000-0000-000000000015', 'user:manage', 'Manage Users', 'user', 'manage'),
  ('30000000-0000-0000-0000-000000000016', 'role:manage', 'Manage Roles', 'role', 'manage');

-- Business App
INSERT INTO business_apps (id, code, name, description, icon, sort_order, status) VALUES
  ('b0000000-0000-0000-0000-000000000001', 'finance', 'Finance Center', 'Operating data reporting, finance analysis, report review, and archive.', 'chart', 10, 'active');

-- Graph Registry
INSERT INTO graph_registry (id, graph_key, business_app_code, name, version, description, status) VALUES
  ('40000000-0000-0000-0000-000000000001', 'finance_operating_report_graph', 'finance', 'Finance Operating Report Graph', '1.0.0', 'Extract, validate, analyze, and generate finance operating report.', 'active');

-- Workflow Template
INSERT INTO workflow_templates (id, business_app_code, workflow_template_key, name, version, graph_key, definition_json, status) VALUES
  ('50000000-0000-0000-0000-000000000001', 'finance', 'finance_operating_report', 'Operating Data Report', '1.0.0', 'finance_operating_report_graph',
   '{"nodes":[{"id":"upload","type":"file_upload","name":"Upload Operating Data","required":true},{"id":"agent_graph","type":"agent_graph","name":"AI Finance Analysis","graph_key":"finance_operating_report_graph"},{"id":"human_review","type":"human_review","name":"Finance Manager Review","role":"business_reviewer","required":true},{"id":"archive","type":"system","name":"Archive Report","action":"archive_result"}],"edges":[{"from":"upload","to":"agent_graph"},{"from":"agent_graph","to":"human_review","when":"succeeded"},{"from":"human_review","to":"archive","when":"approved"}]}',
   'active');

-- Agent Registry
INSERT INTO agent_registry (id, agent_id, name, domain, reusable_scope, capabilities_json, status) VALUES
  ('60000000-0000-0000-0000-000000000001', 'data_extract_agent', 'Data Extract Agent', 'shared', 'shared', '["extract_table","parse_csv","parse_excel"]', 'active'),
  ('60000000-0000-0000-0000-000000000002', 'schema_mapping_agent', 'Schema Mapping Agent', 'shared', 'shared', '["normalize_fields","map_schema"]', 'active'),
  ('60000000-0000-0000-0000-000000000003', 'validation_agent', 'Validation Agent', 'shared', 'shared', '["validate_required_fields","detect_outliers"]', 'active'),
  ('60000000-0000-0000-0000-000000000004', 'finance_analysis_agent', 'Finance Analysis Agent', 'finance', 'domain_only', '["metric_analysis","trend_summary","risk_explanation"]', 'active'),
  ('60000000-0000-0000-0000-000000000005', 'report_agent', 'Report Agent', 'shared', 'shared', '["report_generation","summary_generation"]', 'active'),
  ('60000000-0000-0000-0000-000000000006', 'review_summary_agent', 'Review Summary Agent', 'shared', 'shared', '["review_summary","warning_summary"]', 'active');

-- Tool Registry
INSERT INTO tool_registry (id, tool_id, name, domain, risk_level, is_shared, status) VALUES
  ('70000000-0000-0000-0000-000000000001', 'parse_csv', 'Parse CSV', 'shared', 'low', true, 'active'),
  ('70000000-0000-0000-0000-000000000002', 'parse_excel', 'Parse Excel', 'shared', 'low', true, 'active'),
  ('70000000-0000-0000-0000-000000000003', 'normalize_finance_schema', 'Normalize Finance Schema', 'finance', 'low', false, 'active'),
  ('70000000-0000-0000-0000-000000000004', 'validate_finance_metrics', 'Validate Finance Metrics', 'finance', 'medium', false, 'active'),
  ('70000000-0000-0000-0000-000000000005', 'generate_finance_report', 'Generate Finance Report', 'finance', 'medium', false, 'active'),
  ('70000000-0000-0000-0000-000000000006', 'archive_report', 'Archive Report', 'finance', 'high', false, 'active');

-- Agent Tool Permissions
INSERT INTO agent_tool_permissions (id, agent_id, tool_id, business_app_code, status) VALUES
  ('80000000-0000-0000-0000-000000000001', 'data_extract_agent', 'parse_csv', 'finance', 'active'),
  ('80000000-0000-0000-0000-000000000002', 'data_extract_agent', 'parse_excel', 'finance', 'active'),
  ('80000000-0000-0000-0000-000000000003', 'schema_mapping_agent', 'normalize_finance_schema', 'finance', 'active'),
  ('80000000-0000-0000-0000-000000000004', 'validation_agent', 'validate_finance_metrics', 'finance', 'active'),
  ('80000000-0000-0000-0000-000000000005', 'finance_analysis_agent', 'validate_finance_metrics', 'finance', 'active'),
  ('80000000-0000-0000-0000-000000000006', 'report_agent', 'generate_finance_report', 'finance', 'active'),
  ('80000000-0000-0000-0000-000000000007', 'review_summary_agent', 'generate_finance_report', 'finance', 'active');

-- User-Role assignments
INSERT INTO user_roles (user_id, role_id) VALUES
  ('10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001'),
  ('10000000-0000-0000-0000-000000000002', '20000000-0000-0000-0000-000000000002'),
  ('10000000-0000-0000-0000-000000000003', '20000000-0000-0000-0000-000000000003'),
  ('10000000-0000-0000-0000-000000000004', '20000000-0000-0000-0000-000000000004');

-- Role-Permission mappings
-- platform_admin: all permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000001', id FROM permissions;

-- business_user
INSERT INTO role_permissions (role_id, permission_id) VALUES
  ('20000000-0000-0000-0000-000000000002', '30000000-0000-0000-0000-000000000001'),
  ('20000000-0000-0000-0000-000000000002', '30000000-0000-0000-0000-000000000002'),
  ('20000000-0000-0000-0000-000000000002', '30000000-0000-0000-0000-000000000003'),
  ('20000000-0000-0000-0000-000000000002', '30000000-0000-0000-0000-000000000004'),
  ('20000000-0000-0000-0000-000000000002', '30000000-0000-0000-0000-000000000005'),
  ('20000000-0000-0000-0000-000000000002', '30000000-0000-0000-0000-000000000008'),
  ('20000000-0000-0000-0000-000000000002', '30000000-0000-0000-0000-000000000009');

-- business_reviewer
INSERT INTO role_permissions (role_id, permission_id) VALUES
  ('20000000-0000-0000-0000-000000000003', '30000000-0000-0000-0000-000000000001'),
  ('20000000-0000-0000-0000-000000000003', '30000000-0000-0000-0000-000000000002'),
  ('20000000-0000-0000-0000-000000000003', '30000000-0000-0000-0000-000000000004'),
  ('20000000-0000-0000-0000-000000000003', '30000000-0000-0000-0000-000000000009'),
  ('20000000-0000-0000-0000-000000000003', '30000000-0000-0000-0000-000000000010'),
  ('20000000-0000-0000-0000-000000000003', '30000000-0000-0000-0000-000000000011');

-- ops_viewer
INSERT INTO role_permissions (role_id, permission_id) VALUES
  ('20000000-0000-0000-0000-000000000004', '30000000-0000-0000-0000-000000000001'),
  ('20000000-0000-0000-0000-000000000004', '30000000-0000-0000-0000-000000000002'),
  ('20000000-0000-0000-0000-000000000004', '30000000-0000-0000-0000-000000000004'),
  ('20000000-0000-0000-0000-000000000004', '30000000-0000-0000-0000-000000000007'),
  ('20000000-0000-0000-0000-000000000004', '30000000-0000-0000-0000-000000000009'),
  ('20000000-0000-0000-0000-000000000004', '30000000-0000-0000-0000-000000000010'),
  ('20000000-0000-0000-0000-000000000004', '30000000-0000-0000-0000-000000000012');

-- Domain Policy
INSERT INTO domain_policies (id, business_app_code, allowed_agent_domains, allowed_tool_domains, allow_shared_agents, allow_shared_tools, high_risk_requires_review, status) VALUES
  ('90000000-0000-0000-0000-000000000001', 'finance', '["finance","shared"]', '["finance","shared"]', true, true, true, 'active');
