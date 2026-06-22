package file

import (
	"context"

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
		 (id, workflow_instance_id, business_app_code, storage_bucket, storage_key,
		  original_filename, content_type, size_bytes, file_role, uploaded_by, checksum)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 RETURNING id, created_at, updated_at`,
		f.ID, f.WorkflowInstanceID, f.BusinessAppCode, f.StorageBucket, f.StorageKey,
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
	f := &File{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, workflow_instance_id, business_app_code, storage_bucket, storage_key,
		        original_filename, content_type, size_bytes, file_role, uploaded_by,
		        checksum, created_at, updated_at
		 FROM files WHERE id = $1`,
		id,
	).Scan(&f.ID, &f.WorkflowInstanceID, &f.BusinessAppCode, &f.StorageBucket, &f.StorageKey,
		&f.OriginalFilename, &f.ContentType, &f.SizeBytes, &f.FileRole, &f.UploadedBy,
		&f.Checksum, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return f, nil
}
