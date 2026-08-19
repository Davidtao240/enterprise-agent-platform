-- M8-D: Knowledge Base v1 — collections + document processing status

CREATE TABLE knowledge_collections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(256) NOT NULL,
    description TEXT,
    embedding_model VARCHAR(128) NOT NULL DEFAULT 'text-embedding-3-small',
    chunk_size INTEGER NOT NULL DEFAULT 1000,
    chunk_overlap INTEGER NOT NULL DEFAULT 100,
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'inactive')),
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_kb_collections_tenant ON knowledge_collections(tenant_id, status);

-- Add collection_id to knowledge_documents
ALTER TABLE knowledge_documents ADD COLUMN collection_id UUID REFERENCES knowledge_collections(id);

CREATE INDEX idx_kb_docs_collection ON knowledge_documents(collection_id, status);

-- Grant permissions
GRANT SELECT, INSERT, UPDATE, DELETE ON knowledge_collections TO "platform_admin";
GRANT SELECT, INSERT, UPDATE ON knowledge_collections TO "finance_manager";
GRANT SELECT ON knowledge_collections TO "finance_user";
GRANT SELECT ON knowledge_collections TO "ops_viewer";

GRANT SELECT, INSERT, UPDATE ON knowledge_documents TO "platform_admin";
GRANT SELECT, INSERT ON knowledge_documents TO "finance_manager";
GRANT SELECT ON knowledge_documents TO "finance_user";

GRANT SELECT, INSERT, UPDATE ON knowledge_chunks TO "platform_admin";
GRANT SELECT, INSERT ON knowledge_chunks TO "finance_manager";
GRANT SELECT ON knowledge_chunks TO "finance_user";