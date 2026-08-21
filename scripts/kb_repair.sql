-- Knowledge Base schema repair: align tables with go repository.go expectations.
-- All three tables are empty (verified), safe to recreate.

CREATE EXTENSION IF NOT EXISTS vector;

DROP TABLE IF EXISTS knowledge_chunks;
DROP TABLE IF EXISTS knowledge_documents;
DROP TABLE IF EXISTS knowledge_collections;

CREATE TABLE knowledge_collections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    embedding_model VARCHAR(128) NOT NULL DEFAULT 'text-embedding-3-small',
    chunk_size INTEGER NOT NULL DEFAULT 1000,
    chunk_overlap INTEGER NOT NULL DEFAULT 100,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_knowledge_collections_tenant ON knowledge_collections(tenant_id);

CREATE TABLE knowledge_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    collection_id UUID REFERENCES knowledge_collections(id) ON DELETE CASCADE,
    file_name VARCHAR(512) NOT NULL,
    file_path VARCHAR(1024) NOT NULL DEFAULT '',
    file_size BIGINT NOT NULL DEFAULT 0,
    mime_type VARCHAR(128) NOT NULL DEFAULT 'application/octet-stream',
    status VARCHAR(32) NOT NULL DEFAULT 'processing',
    chunk_count INTEGER NOT NULL DEFAULT 0,
    processed_at TIMESTAMPTZ,
    error_message TEXT,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_knowledge_documents_tenant ON knowledge_documents(tenant_id);
CREATE INDEX idx_knowledge_documents_collection ON knowledge_documents(collection_id, status);

CREATE TABLE knowledge_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL,
    chunk_index INTEGER NOT NULL,
    content TEXT NOT NULL,
    embedding vector(1536),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (document_id, chunk_index)
);

CREATE INDEX idx_knowledge_chunks_document ON knowledge_chunks(document_id);
CREATE INDEX idx_knowledge_chunks_embedding ON knowledge_chunks USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 200);
