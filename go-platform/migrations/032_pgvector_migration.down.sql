-- M7-D: pgvector migration — down

DROP INDEX IF EXISTS idx_knowledge_chunks_embedding;
DROP INDEX IF EXISTS idx_knowledge_chunks_tenant_doc;
DROP TABLE IF EXISTS knowledge_chunks;
DROP INDEX IF EXISTS idx_knowledge_docs_tenant_status;
DROP TABLE IF EXISTS knowledge_documents;