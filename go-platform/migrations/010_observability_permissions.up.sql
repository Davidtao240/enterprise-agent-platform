-- V2.3: protected production-observability access.
INSERT INTO permissions (id, code, name, resource, action) VALUES
  ('30000000-0000-0000-0000-000000000019', 'observability:read', 'Read Platform Observability', 'observability', 'read')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT role_id, permission_id
FROM (VALUES
  ('20000000-0000-0000-0000-000000000001'::uuid, '30000000-0000-0000-0000-000000000019'::uuid),
  ('20000000-0000-0000-0000-000000000004'::uuid, '30000000-0000-0000-0000-000000000019'::uuid)
) AS grants(role_id, permission_id)
ON CONFLICT DO NOTHING;
