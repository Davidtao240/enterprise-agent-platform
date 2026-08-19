-- M7-A: Conversation Engine — user-facing chat wrapper around agent_threads / agent_runs.

CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    created_by UUID NOT NULL REFERENCES users(id),
    thread_id UUID NOT NULL REFERENCES agent_threads(id),
    agent_package_code VARCHAR(64) NOT NULL,
    title VARCHAR(256),
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'closed', 'archived')),
    budget_json JSONB,
    last_message_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_conversations_thread ON conversations(thread_id);
CREATE INDEX idx_conversations_tenant_user ON conversations(tenant_id, created_by, status);
CREATE INDEX idx_conversations_package_time ON conversations(agent_package_code, last_message_at DESC);

CREATE TABLE conversation_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    run_id UUID REFERENCES agent_runs(id),
    role VARCHAR(16) NOT NULL
        CHECK (role IN ('user', 'assistant', 'system', 'tool')),
    content TEXT NOT NULL,
    seq INTEGER NOT NULL,
    tokens INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_messages_conv_seq ON conversation_messages(conversation_id, seq);

-- conversation:read, conversation:write permissions
INSERT INTO permissions (id, code, name, resource, action) VALUES
    ('31000000-0000-0000-0000-000000000030', 'conversation:read', 'Read Conversations', 'conversation', 'read'),
    ('31000000-0000-0000-0000-000000000031', 'conversation:write', 'Write Conversations', 'conversation', 'write')
ON CONFLICT (code) DO NOTHING;

-- grant to all default roles
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.role_id, p.permission_id
FROM (VALUES
    ('20000000-0000-0000-0000-000000000001'::uuid),
    ('20000000-0000-0000-0000-000000000002'::uuid),
    ('20000000-0000-0000-0000-000000000003'::uuid),
    ('20000000-0000-0000-0000-000000000004'::uuid)
) AS r(role_id),
(VALUES
    ('31000000-0000-0000-0000-000000000030'::uuid),
    ('31000000-0000-0000-0000-000000000031'::uuid)
) AS p(permission_id)
ON CONFLICT DO NOTHING;