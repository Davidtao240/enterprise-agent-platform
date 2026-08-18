package memory

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrMemoryNotFound 记忆不存在或不属于该租户。
var ErrMemoryNotFound = errors.New("memory not found")

const memorySelect = `SELECT id, tenant_id, scope, scope_id, content, acl, created_by,
	expires_at, created_at, updated_at, deleted_at
	FROM agent_memory`

// Repository 封装 agent_memory 表访问。
// 所有查询强制 tenant_id 过滤(跨租户隔离);
// 检索路径自动排除软删除与已过期条目。
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository 创建 Repository。
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create 写入一条记忆。
func (r *Repository) Create(ctx context.Context, req *WriteRequest) (*Memory, error) {
	id := uuid.NewString()
	now := time.Now().UTC()

	aclJSON, err := json.Marshal(req.ACL)
	if err != nil {
		return nil, err
	}
	var expiresAt any
	if req.ExpiresAt != nil {
		expiresAt = *req.ExpiresAt
	}

	row := r.pool.QueryRow(ctx, `
		INSERT INTO agent_memory
			(id, scope, scope_id, tenant_id, content, acl, created_by, expires_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)
		RETURNING id, tenant_id, scope, scope_id, content, acl, created_by,
			expires_at, created_at, updated_at, deleted_at`,
		id, req.Scope, req.ScopeID, req.TenantID, req.Content, aclJSON, req.CreatedBy, expiresAt, now,
	)
	return scanMemory(row)
}

// ListActive 按 (tenant, scope, scope_id) 列出有效记忆
// (未软删除且未过期),按创建时间正序。ACL 过滤由 Service 层执行。
func (r *Repository) ListActive(ctx context.Context, tenantID, scope, scopeID string, limit int) ([]*Memory, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, memorySelect+`
		WHERE tenant_id = $1 AND scope = $2 AND scope_id = $3
		  AND deleted_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at ASC
		LIMIT $4`, tenantID, scope, scopeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetByID 按租户 + ID 查找(租户隔离)。
func (r *Repository) GetByID(ctx context.Context, tenantID, id string) (*Memory, error) {
	row := r.pool.QueryRow(ctx, memorySelect+`
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, id, tenantID)
	return scanMemory(row)
}

// SoftDelete 软删除(租户隔离)。
func (r *Repository) SoftDelete(ctx context.Context, tenantID, id string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE agent_memory SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrMemoryNotFound
	}
	return nil
}

func scanMemory(row pgx.Row) (*Memory, error) {
	var (
		m         Memory
		aclJSON   []byte
		content   []byte
		deletedAt *time.Time
	)
	if err := row.Scan(&m.ID, &m.TenantID, &m.Scope, &m.ScopeID, &content, &aclJSON,
		&m.CreatedBy, &m.ExpiresAt, &m.CreatedAt, &m.UpdatedAt, &deletedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMemoryNotFound
		}
		return nil, err
	}
	m.ContentJSON = append(json.RawMessage(nil), content...)
	if len(aclJSON) > 0 && string(aclJSON) != "null" {
		if err := json.Unmarshal(aclJSON, &m.ACL); err != nil {
			return nil, err
		}
	}
	m.DeletedAt = deletedAt
	return &m, nil
}
