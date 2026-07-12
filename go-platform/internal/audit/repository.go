package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 封装 audit_logs 表的数据库查询。
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository 创建 Repository 实例。
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// InsertLog 写入一条审计日志，返回新记录的 ID 和创建时间。
func (r *Repository) InsertLog(ctx context.Context, entry AuditLogEntry) (string, time.Time, error) {
	var id string
	var createdAt time.Time
	err := r.pool.QueryRow(ctx,
		`INSERT INTO audit_logs (trace_id, tenant_id, actor_user_id, business_app_code, action, resource_type, resource_id, status, detail_json, ip_address, user_agent)
		 VALUES ($1, COALESCE(NULLIF($2, '')::uuid, '00000000-0000-0000-0000-000000000010'), $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id, created_at`,
		entry.TraceID, entry.TenantID, entry.ActorUserID, entry.BusinessAppCode, entry.Action,
		entry.ResourceType, entry.ResourceID, entry.Status, entry.DetailJSON, entry.IPAddress, entry.UserAgent,
	).Scan(&id, &createdAt)
	if err != nil {
		return id, createdAt, fmt.Errorf("insert audit log: %w", err)
	}
	return id, createdAt, nil
}

// ListParams 审计日志查询参数。
type ListParams struct {
	TenantID        string
	TraceID         string
	BusinessAppCode string
	Action          string
	ActorUserID     string
	ResourceType    string
	ResourceID      string
	Status          string
	CreatedFrom     string
	CreatedTo       string
	Page            int
	PageSize        int
}

// ListLogs 分页查询审计日志，支持按业务域、操作、用户、资源类型过滤。
func (r *Repository) ListLogs(ctx context.Context, p ListParams) ([]AuditLog, int, error) {
	whereClause, args, argIdx := buildWhereClause(p)

	// Count total
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs %s", whereClause)
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	// Fetch page
	offset := (p.Page - 1) * p.PageSize
	dataQuery := fmt.Sprintf(
		`SELECT id, trace_id, actor_user_id, business_app_code, action, resource_type, resource_id, status, detail_json, ip_address, user_agent, created_at
		 FROM audit_logs %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		whereClause, argIdx, argIdx+1,
	)
	args = append(args, p.PageSize, offset)

	logs, err := r.queryLogs(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (r *Repository) Stats(ctx context.Context, p ListParams) (*StatsResponse, error) {
	whereClause, args, argIdx := buildWhereClause(p)

	resp := &StatsResponse{}
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_logs "+whereClause, args...).Scan(&resp.Total); err != nil {
		return nil, fmt.Errorf("count audit logs: %w", err)
	}

	statusBuckets, err := r.countBuckets(ctx, "status", whereClause, args)
	if err != nil {
		return nil, err
	}
	resp.ByStatus = statusBuckets

	actionBuckets, err := r.countBuckets(ctx, "action", whereClause, args)
	if err != nil {
		return nil, err
	}
	resp.ByAction = actionBuckets

	recentQuery := fmt.Sprintf(
		`SELECT id, trace_id, actor_user_id, business_app_code, action, resource_type, resource_id, status, detail_json, ip_address, user_agent, created_at
		 FROM audit_logs %s ORDER BY created_at DESC LIMIT $%d`,
		whereClause, argIdx,
	)
	recentArgs := append(append([]any{}, args...), 10)
	recent, err := r.queryLogs(ctx, recentQuery, recentArgs...)
	if err != nil {
		return nil, err
	}
	resp.RecentKeyActions = toListResponse(recent)
	return resp, nil
}

func buildWhereClause(p ListParams) (string, []any, int) {
	var conditions []string
	var args []any
	argIdx := 1
	if p.TenantID != "" {
		conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
		args = append(args, p.TenantID)
		argIdx++
	}

	if p.TraceID != "" {
		conditions = append(conditions, fmt.Sprintf("trace_id = $%d", argIdx))
		args = append(args, p.TraceID)
		argIdx++
	}
	if p.BusinessAppCode != "" {
		conditions = append(conditions, fmt.Sprintf("business_app_code = $%d", argIdx))
		args = append(args, p.BusinessAppCode)
		argIdx++
	}
	if p.Action != "" {
		conditions = append(conditions, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, p.Action)
		argIdx++
	}
	if p.ActorUserID != "" {
		conditions = append(conditions, fmt.Sprintf("actor_user_id = $%d", argIdx))
		args = append(args, p.ActorUserID)
		argIdx++
	}
	if p.ResourceType != "" {
		conditions = append(conditions, fmt.Sprintf("resource_type = $%d", argIdx))
		args = append(args, p.ResourceType)
		argIdx++
	}
	if p.ResourceID != "" {
		conditions = append(conditions, fmt.Sprintf("resource_id = $%d", argIdx))
		args = append(args, p.ResourceID)
		argIdx++
	}
	if p.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, p.Status)
		argIdx++
	}
	if p.CreatedFrom != "" {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d::timestamptz", argIdx))
		args = append(args, p.CreatedFrom)
		argIdx++
	}
	if p.CreatedTo != "" {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d::timestamptz", argIdx))
		args = append(args, p.CreatedTo)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + conditions[0]
		for _, c := range conditions[1:] {
			whereClause += " AND " + c
		}
	}
	return whereClause, args, argIdx
}

func (r *Repository) queryLogs(ctx context.Context, query string, args ...any) ([]AuditLog, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit logs: %w", err)
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.TraceID, &l.ActorUserID, &l.BusinessAppCode,
			&l.Action, &l.ResourceType, &l.ResourceID, &l.Status,
			&l.DetailJSON, &l.IPAddress, &l.UserAgent, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (r *Repository) countBuckets(ctx context.Context, column string, whereClause string, args []any) ([]CountBucket, error) {
	query := fmt.Sprintf(
		`SELECT %s, COUNT(*) FROM audit_logs %s GROUP BY %s ORDER BY COUNT(*) DESC, %s LIMIT 20`,
		column, whereClause, column, column,
	)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit %s buckets: %w", column, err)
	}
	defer rows.Close()

	var buckets []CountBucket
	for rows.Next() {
		var b CountBucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			return nil, err
		}
		buckets = append(buckets, b)
	}
	return buckets, rows.Err()
}

func toListResponse(logs []AuditLog) []ListResponse {
	items := make([]ListResponse, len(logs))
	for i, l := range logs {
		items[i] = ListResponse{
			ID:              l.ID,
			TraceID:         l.TraceID,
			ActorUserID:     l.ActorUserID,
			BusinessAppCode: l.BusinessAppCode,
			Action:          l.Action,
			ResourceType:    l.ResourceType,
			ResourceID:      l.ResourceID,
			Status:          l.Status,
			DetailJSON:      l.DetailJSON,
			CreatedAt:       l.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}
	return items
}
