package governance

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) List(ctx context.Context, tenantID, resourceType, resourceKey, status string) ([]ConfigurationVersion, error) {
	conditions := []string{"1=1"}
	args := []any{}
	if tenantID != "" {
		conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", len(args)+1))
		args = append(args, tenantID)
	}
	if resourceType != "" {
		conditions = append(conditions, fmt.Sprintf("resource_type = $%d", len(args)+1))
		args = append(args, resourceType)
	}
	if resourceKey != "" {
		conditions = append(conditions, fmt.Sprintf("resource_key = $%d", len(args)+1))
		args = append(args, resourceKey)
	}
	if status != "" {
		conditions = append(conditions, fmt.Sprintf("lifecycle_status = $%d", len(args)+1))
		args = append(args, status)
	}
	rows, err := r.pool.Query(ctx, `SELECT id, tenant_id, resource_type, resource_key, version, lifecycle_status, snapshot_json,
		change_summary, created_by, approved_by, approved_at, published_at, deprecated_at, trace_id, created_at, updated_at
		FROM configuration_versions WHERE `+strings.Join(conditions, " AND ")+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ConfigurationVersion{}
	for rows.Next() {
		var item ConfigurationVersion
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ResourceType, &item.ResourceKey, &item.Version, &item.LifecycleStatus, &item.SnapshotJSON, &item.ChangeSummary, &item.CreatedBy, &item.ApprovedBy, &item.ApprovedAt, &item.PublishedAt, &item.DeprecatedAt, &item.TraceID, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Create(ctx context.Context, item *ConfigurationVersion) error {
	return r.pool.QueryRow(ctx, `INSERT INTO configuration_versions (tenant_id, resource_type, resource_key, version, lifecycle_status, snapshot_json, change_summary, created_by, trace_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, created_at, updated_at`, item.TenantID, item.ResourceType, item.ResourceKey, item.Version, item.LifecycleStatus, item.SnapshotJSON, item.ChangeSummary, item.CreatedBy, item.TraceID).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
}

func (r *Repository) FindByID(ctx context.Context, tenantID, id string) (*ConfigurationVersion, error) {
	var item ConfigurationVersion
	err := r.pool.QueryRow(ctx, `SELECT id, tenant_id, resource_type, resource_key, version, lifecycle_status, snapshot_json,
		change_summary, created_by, approved_by, approved_at, published_at, deprecated_at, trace_id, created_at, updated_at
		FROM configuration_versions WHERE tenant_id = $1 AND id = $2`, tenantID, id).Scan(&item.ID, &item.TenantID, &item.ResourceType, &item.ResourceKey, &item.Version, &item.LifecycleStatus, &item.SnapshotJSON, &item.ChangeSummary, &item.CreatedBy, &item.ApprovedBy, &item.ApprovedAt, &item.PublishedAt, &item.DeprecatedAt, &item.TraceID, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) Transition(ctx context.Context, tenantID, id, from, to string, actorID *string) (bool, error) {
	sets := []string{"lifecycle_status = $1", "updated_at = now()"}
	args := []any{to}
	if to == StatusPublished {
		sets = append(sets, "published_at = now()")
	}
	if to == StatusDeprecated {
		sets = append(sets, "deprecated_at = now()")
	}
	if to == StatusPublished && actorID != nil {
		sets = append(sets, fmt.Sprintf("approved_by = $%d", len(args)+1), "approved_at = now()")
		args = append(args, *actorID)
	}
	args = append(args, tenantID, id, from)
	result, err := r.pool.Exec(ctx, `UPDATE configuration_versions SET `+strings.Join(sets, ", ")+fmt.Sprintf(" WHERE tenant_id = $%d AND id = $%d AND lifecycle_status = $%d", len(args)-2, len(args)-1, len(args)), args...)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}
