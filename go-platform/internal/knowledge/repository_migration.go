package knowledge

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
)

// EnsureCollectionsTable 幂等地将 knowledge_collections / knowledge_documents /
// knowledge_chunks 对齐到 repository.go 期望的结构。
// 仅在表缺失时创建、缺列时补列；不删除任何已有数据。
func EnsureCollectionsTable(ctx context.Context, db interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
}) error {
	createSQL := `
	CREATE TABLE IF NOT EXISTS knowledge_collections (
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

	ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS embedding_model VARCHAR(128) NOT NULL DEFAULT 'text-embedding-3-small';
	ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS chunk_size INTEGER NOT NULL DEFAULT 1000;
	ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS chunk_overlap INTEGER NOT NULL DEFAULT 100;
	ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS status VARCHAR(32) NOT NULL DEFAULT 'active';
	ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS created_by UUID;
	ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

	CREATE TABLE IF NOT EXISTS knowledge_documents (
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

	ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS collection_id UUID;
	ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS file_name VARCHAR(512);
	ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS file_path VARCHAR(1024) NOT NULL DEFAULT '';
	ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS file_size BIGINT NOT NULL DEFAULT 0;
	ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS mime_type VARCHAR(128) NOT NULL DEFAULT 'application/octet-stream';
	ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS status VARCHAR(32) NOT NULL DEFAULT 'processing';
	ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS chunk_count INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS processed_at TIMESTAMPTZ;
	ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS error_message TEXT;
	ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS created_by UUID;
	`
	if _, err := db.Exec(ctx, createSQL); err != nil {
		return err
	}

	// knowledge_chunks 依赖 pgvector 的 vector 类型；扩展不可用时跳过建表，
	// 由 scripts/kb_repair.sql 在启用 pgvector 后补建。
	chunksSQL := `
	CREATE EXTENSION IF NOT EXISTS vector;

	CREATE TABLE IF NOT EXISTS knowledge_chunks (
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

	CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_document ON knowledge_chunks(document_id);
	`
	_, err := db.Exec(ctx, chunksSQL)
	return err
}
