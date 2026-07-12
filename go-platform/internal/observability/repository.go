package observability

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository struct{ db querier }

func NewRepository(db querier) *Repository { return &Repository{db: db} }

func (r *Repository) Summary(ctx context.Context, tenantID string) (*Summary, error) {
	summary := &Summary{}
	var err error
	if summary.WorkflowByStatus, err = r.countBuckets(ctx, "workflow_instances", "status", "deleted_at IS NULL AND tenant_id = $1", tenantID); err != nil {
		return nil, err
	}
	if summary.AgentRunByStatus, err = r.countBuckets(ctx, "agent_run_logs", "status", "tenant_id = $1", tenantID); err != nil {
		return nil, err
	}
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM agent_run_logs WHERE tenant_id = $1 AND status = 'failed' AND created_at >= now() - interval '24 hours'`, tenantID).Scan(&summary.FailedRunsLast24h); err != nil {
		return nil, err
	}
	if err := r.db.QueryRow(ctx, `SELECT COALESCE(AVG(duration_ms), 0)::int FROM agent_run_logs WHERE tenant_id = $1 AND duration_ms IS NOT NULL`, tenantID).Scan(&summary.AverageDurationMs); err != nil {
		return nil, err
	}
	if err := r.db.QueryRow(ctx, `SELECT COALESCE(SUM(COALESCE((usage_json->>'cost')::numeric, 0)), 0)::float8 FROM agent_run_logs WHERE tenant_id = $1`, tenantID).Scan(&summary.TotalReportedCost); err != nil {
		return nil, err
	}
	return summary, nil
}

func (r *Repository) countBuckets(ctx context.Context, table, column, where, tenantID string) ([]CountBucket, error) {
	rows, err := r.db.Query(ctx, fmt.Sprintf(`SELECT %s, COUNT(*) FROM %s WHERE %s GROUP BY %s ORDER BY COUNT(*) DESC, %s`, column, table, where, column, column), tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []CountBucket{}
	for rows.Next() {
		var item CountBucket
		if err := rows.Scan(&item.Key, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
