package knowledge

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Collection struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	EmbeddingModel string    `json:"embedding_model"`
	ChunkSize      int       `json:"chunk_size"`
	ChunkOverlap   int       `json:"chunk_overlap"`
	Status         string    `json:"status"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Document struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	CollectionID string    `json:"collection_id"`
	FileName     string    `json:"file_name"`
	FilePath     string    `json:"file_path"`
	FileSize     int64     `json:"file_size"`
	MimeType     string    `json:"mime_type"`
	Content      string    `json:"-"`
	Status       string    `json:"status"`
	ChunkCount   int       `json:"chunk_count"`
	ProcessedAt  time.Time `json:"processed_at,omitempty"`
	ErrorMsg     string    `json:"error_message,omitempty"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Chunk struct {
	ID         string    `json:"id"`
	DocumentID string    `json:"document_id"`
	TenantID   string    `json:"tenant_id"`
	ChunkIndex int       `json:"chunk_index"`
	Content    string    `json:"content"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type SearchRequest struct {
	CollectionID string  `json:"collection_id"`
	Query       string  `json:"query"`
	TopK        int     `json:"top_k"`
	MinScore    float64 `json:"min_score"`
}

type SearchResult struct {
	ChunkID    string  `json:"chunk_id"`
	DocumentID string  `json:"document_id"`
	FileName   string  `json:"file_name"`
	ChunkIndex int     `json:"chunk_index"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	Collection string  `json:"collection"`
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ── Collection CRUD ──────────────────────────────────────────

func (r *Repository) CreateCollection(ctx context.Context, c *Collection) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO knowledge_collections
		 (id, tenant_id, name, description, embedding_model, chunk_size, chunk_overlap, status, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 RETURNING created_at, updated_at`,
		c.ID, c.TenantID, c.Name, c.Description, c.EmbeddingModel,
		c.ChunkSize, c.ChunkOverlap, c.Status, c.CreatedBy,
	).Scan(&c.CreatedAt, &c.UpdatedAt)
}

func (r *Repository) ListCollections(ctx context.Context, tenantID string) ([]Collection, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, name, description, embedding_model, chunk_size, chunk_overlap, status,
		        created_by, created_at, updated_at
		 FROM knowledge_collections
		 WHERE tenant_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at DESC`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Name, &c.Description,
			&c.EmbeddingModel, &c.ChunkSize, &c.ChunkOverlap, &c.Status,
			&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, nil
}

func (r *Repository) GetCollection(ctx context.Context, id, tenantID string) (*Collection, error) {
	var c Collection
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, description, embedding_model, chunk_size, chunk_overlap, status,
		        created_by, created_at, updated_at
		 FROM knowledge_collections
		 WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`,
		id, tenantID,
	).Scan(&c.ID, &c.TenantID, &c.Name, &c.Description,
		&c.EmbeddingModel, &c.ChunkSize, &c.ChunkOverlap, &c.Status,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &c, err
}

func (r *Repository) DeleteCollection(ctx context.Context, id, tenantID string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE knowledge_collections SET deleted_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("collection not found")
	}
	return nil
}

// ── Document CRUD ──────────────────────────────────────────

func (r *Repository) CreateDocument(ctx context.Context, d *Document) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO knowledge_documents
		 (id, tenant_id, collection_id, file_name, file_path, file_size, mime_type, status, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 RETURNING created_at, updated_at`,
		d.ID, d.TenantID, d.CollectionID, d.FileName, d.FilePath,
		d.FileSize, d.MimeType, d.Status, d.CreatedBy,
	).Scan(&d.CreatedAt, &d.UpdatedAt)
}

func (r *Repository) UpdateDocument(ctx context.Context, d *Document) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE knowledge_documents
		 SET status = $2, chunk_count = $3, error_message = $4,
		     processed_at = $5, updated_at = NOW()
		 WHERE id = $1`,
		d.ID, d.Status, d.ChunkCount, d.ErrorMsg, d.ProcessedAt)
	return err
}

func (r *Repository) ListDocuments(ctx context.Context, tenantID, collectionID, status string) ([]Document, error) {
	query := `SELECT id, tenant_id, COALESCE(collection_id::text,''), file_name, file_path,
			     file_size, mime_type, status, chunk_count,
			     COALESCE(processed_at, NOW()), COALESCE(error_message,''),
			     created_by, created_at, updated_at
			 FROM knowledge_documents
			 WHERE tenant_id = $1`
	args := []any{tenantID}
	argIdx := 2

	if collectionID != "" {
		query += fmt.Sprintf(" AND collection_id = $%d", argIdx)
		args = append(args, collectionID)
		argIdx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocumentRows(rows)
}

// ListProcessingDocuments 返回所有租户中仍处于 processing 状态的文档。
// 仅供启动恢复扫描使用（跨租户读取），不暴露给任何请求处理器。
func (r *Repository) ListProcessingDocuments(ctx context.Context) ([]Document, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, COALESCE(collection_id::text,''), file_name, file_path,
			     file_size, mime_type, status, chunk_count,
			     COALESCE(processed_at, NOW()), COALESCE(error_message,''),
			     created_by, created_at, updated_at
			 FROM knowledge_documents
			 WHERE status = 'processing'
			 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocumentRows(rows)
}

func scanDocumentRows(rows pgx.Rows) ([]Document, error) {
	var items []Document
	for rows.Next() {
		var d Document
		var collID string
		var procAt time.Time
		var errMsg string
		if err := rows.Scan(&d.ID, &d.TenantID, &collID, &d.FileName, &d.FilePath,
			&d.FileSize, &d.MimeType, &d.Status, &d.ChunkCount,
			&procAt, &errMsg, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		if collID != "" {
			d.CollectionID = collID
		}
		d.ProcessedAt = procAt
		d.ErrorMsg = errMsg
		items = append(items, d)
	}
	return items, rows.Err()
}

func (r *Repository) DeleteDocument(ctx context.Context, id, tenantID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM knowledge_chunks WHERE document_id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`DELETE FROM knowledge_documents WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	return err
}

// ── Chunk operations ──────────────────────────────────────────

func (r *Repository) InsertChunks(ctx context.Context, chunks []Chunk, embeddingModel string) error {
	if len(chunks) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, c := range chunks {
		batch.Queue(
			`INSERT INTO knowledge_chunks
			 (id, document_id, tenant_id, chunk_index, content, metadata, embedding)
			 VALUES ($1,$2,$3,$4,$5,$6,$7::vector)
			 ON CONFLICT (document_id, chunk_index) DO NOTHING`,
			c.ID, c.DocumentID, c.TenantID, c.ChunkIndex, c.Content, c.Metadata,
			nil, // embedding will be updated separately
		)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range chunks {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
	}
	return nil
}

func (r *Repository) UpdateChunkEmbeddings(ctx context.Context, chunkIDs []string, embeddings [][]float32) error {
	for i, id := range chunkIDs {
		if i >= len(embeddings) {
			break
		}
		embText := formatVector(embeddings[i])
		_, err := r.pool.Exec(ctx,
			`UPDATE knowledge_chunks SET embedding = $2::vector WHERE id = $1`,
			id, embText)
		if err != nil {
			return fmt.Errorf("update chunk %s embedding: %w", id, err)
		}
	}
	return nil
}

// ── Vector search ──────────────────────────────────────────

func (r *Repository) SearchByVector(ctx context.Context, tenantID, collectionID string, queryEmbedding []float32, topK int, minScore float64) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	embText := formatVector(queryEmbedding)
	// <=> 是 pgvector 的余弦距离操作符；score = 1 - cos_dist ∈ [-1,1]，语义为相似度。
	// $1 必须显式 ::vector cast：pgx 把 Go string 推断为 text，
	// 否则 vector <=> text 无隐式转换导致查询报错。
	const distOperator = "<=>"

	args := []any{embText, tenantID}
	argIdx := 3

	query := `SELECT kc.id, kc.document_id, kd.file_name, kc.chunk_index,
	                 kc.content,
	                 1 - (kc.embedding ` + distOperator + ` $1::vector) AS score,
	                 COALESCE(kc.metadata->>'collection_name', '')
	          FROM knowledge_chunks kc
	          JOIN knowledge_documents kd ON kc.document_id = kd.id
	          WHERE kc.embedding IS NOT NULL
	            AND kd.tenant_id = $2
	            AND kd.status = 'ready'`

	if collectionID != "" {
		query += fmt.Sprintf(" AND kd.collection_id = $%d", argIdx)
		args = append(args, collectionID)
		argIdx++
	}
	if minScore > 0 {
		query += fmt.Sprintf(" AND (1 - (kc.embedding %s $1::vector)) >= $%d", distOperator, argIdx)
		args = append(args, minScore)
		argIdx++
	}

	query += fmt.Sprintf(` ORDER BY kc.embedding %s $1::vector ASC LIMIT $%d`, distOperator, argIdx)
	args = append(args, topK)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var sr SearchResult
		var collectionName string
		if err := rows.Scan(&sr.ChunkID, &sr.DocumentID, &sr.FileName,
			&sr.ChunkIndex, &sr.Content, &sr.Score, &collectionName); err != nil {
			return nil, err
		}
		sr.Collection = collectionName
		results = append(results, sr)
	}
	return results, nil
}

func formatVector(v []float32) string {
	var s string
	for i, f := range v {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf("%f", f)
	}
	return "[" + s + "]"
}