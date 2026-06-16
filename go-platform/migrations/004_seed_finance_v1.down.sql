-- Remove V1 seed data only — tables remain.
DELETE FROM agent_tool_permissions WHERE id LIKE 'atp%';
DELETE FROM tool_registry WHERE id LIKE 't0%';
DELETE FROM agent_registry WHERE id LIKE 'a0%';
DELETE FROM domain_policies WHERE id LIKE 'dp%';
DELETE FROM workflow_templates WHERE id LIKE 'w0%';
DELETE FROM graph_registry WHERE id LIKE 'g0%';
DELETE FROM business_apps WHERE id LIKE 'b0%';
DELETE FROM role_permissions WHERE role_id LIKE 'r0%';
DELETE FROM user_roles WHERE user_id LIKE 'u0%';
DELETE FROM permissions WHERE id LIKE 'p0%';
DELETE FROM roles WHERE id LIKE 'r0%';
DELETE FROM users WHERE id LIKE 'u0%';
DELETE FROM departments WHERE id LIKE 'd0%';
