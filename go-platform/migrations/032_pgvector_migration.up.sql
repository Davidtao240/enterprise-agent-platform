-- M7-D: pgvector migration — 统一向量存储到 PostgreSQL

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE knowledge_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    title VARCHAR(512) NOT NULL,
    source_file_id UUID,
    source_type VARCHAR(32) NOT NULL DEFAULT 'upload'
        CHECK (source_type IN ('upload', 'link', 'manual')),
    content_hash VARCHAR(64),
    status VARCHAR(32) NOT NULL DEFAULT 'parsed'
        CHECK (status IN ('pending', 'parsing', 'parsed', 'indexing', 'ready', 'error', 'deleted')),
    chunk_count INTEGER NOT NULL DEFAULT 0,
    metadata_json JSONB DEFAULT '{}',
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_knowledge_docs_tenant_status ON knowledge_documents(tenant_id, status);

CREATE TABLE knowledge_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    document_id UUID NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    content TEXT NOT NULL,
    embedding vector(1536),
    token_count INTEGER,
    metadata_json JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_knowledge_chunks_tenant_doc ON knowledge_chunks(tenant_id, document_id, chunk_index);
CREATE INDEX idx_knowledge_chunks_embedding ON knowledge_chunks USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 200);