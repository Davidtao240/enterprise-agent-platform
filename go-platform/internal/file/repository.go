package file

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, f *File) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO files
		 (id, tenant_id, workflow_instance_id, business_app_code, storage_bucket, storage_key,
		  original_filename, content_type, size_bytes, file_role, uploaded_by, checksum)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 RETURNING id, created_at, updated_at`,
		f.ID, f.TenantID, f.WorkflowInstanceID, f.BusinessAppCode, f.StorageBucket, f.StorageKey,
		f.OriginalFilename, f.ContentType, f.SizeBytes, f.FileRole, f.UploadedBy, f.Checksum,
	).Scan(&f.ID, &f.CreatedAt, &f.UpdatedAt)
}

func (r *Repository) UpdateStorageKey(ctx context.Context, id, storageKey string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE files SET storage_key = $2, updated_at = now() WHERE id = $1`,
		id, storageKey)
	return err
}

func (r *Repository) FindByID(ctx context.Context, id string) (*File, error) {
	return r.findByIdentifier(ctx, "", id, false)
}

// FindByIdentifierForTenant 按租户 + ID/storage_key 查找文件(租户隔离)。
func (r *Repository) FindByIdentifierForTenant(ctx context.Context, tenantID, identifier string) (*File, error) {
	return r.findByIdentifier(ctx, tenantID, identifier, true)
}

// FindByIdentifier 不带租户过滤,仅限内部服务(受 InternalServiceToken 保护)使用。
func (r *Repository) FindByIdentifier(ctx context.Context, identifier string) (*File, error) {
	return r.findByIdentifier(ctx, "", identifier, true)
}

func (r *Repository) findByIdentifier(ctx context.Context, tenantID, identifier string, allowStorageKey bool) (*File, error) {
	f := &File{}
	where := "id::text = $1"
	args := []any{identifier}
	if allowStorageKey {
		where += " OR storage_key = $1"
	}
	if tenantID != "" {
		where = "(" + where + ") AND tenant_id = $" + strconv.Itoa(len(args)+1)
		args = append(args, tenantID)
	}
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, workflow_instance_id, business_app_code, storage_bucket, storage_key,
		        original_filename, content_type, size_bytes, file_role, uploaded_by,
		        checksum, created_at, updated_at
		 FROM files WHERE `+where+` LIMIT 1`,
		args...,
	).Scan(&f.ID, &f.TenantID, &f.WorkflowInstanceID, &f.BusinessAppCode, &f.StorageBucket, &f.StorageKey,
		&f.OriginalFilename, &f.ContentType, &f.SizeBytes, &f.FileRole, &f.UploadedBy,
		&f.Checksum, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return f, nil
}
