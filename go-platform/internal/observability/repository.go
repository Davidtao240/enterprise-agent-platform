package observability

import (
	"context"
	"fmt"
	"strings"

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

func (r *Repository) GetAVRMetrics(ctx context.Context, tenantID string, days int) (*AVRResponse, error) {
	response := &AVRResponse{
		MetricsByDate: make(map[string]AVRMetrics),
	}

	rows, err := r.db.Query(ctx, `
		SELECT DATE(created_at) AS metric_date,
		       SUM(duration_ms - human_interaction_ms) AS agent_independent_ms,
		       SUM(rework_count) AS rework_total,
		       COUNT(*) AS total_runs
		FROM agent_run_logs
		WHERE tenant_id = $1 AND status = 'success' AND created_at >= now() - MAKE_INTERVAL(DAYS => $2)
		GROUP BY DATE(created_at)
		ORDER BY metric_date DESC
	`, tenantID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type dateMetrics struct {
		date               string
		agentIndependentMs int64
		reworkTotal        int
		totalRuns          int
	}
	var dailyResults []dateMetrics
	for rows.Next() {
		var m dateMetrics
		if err := rows.Scan(&m.date, &m.agentIndependentMs, &m.reworkTotal, &m.totalRuns); err != nil {
			return nil, err
		}
		dailyResults = append(dailyResults, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, d := range dailyResults {
		response.MetricsByDate[d.date] = AVRMetrics{
			AgentIndependentMs: d.agentIndependentMs,
			ReworkTotal:        d.reworkTotal,
			TotalRuns:          d.totalRuns,
		}
	}

	var current AVRMetrics
	if err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_ms - human_interaction_ms), 0)::int8,
		       COALESCE(SUM(rework_count), 0)::int,
		       COUNT(*)::int
		FROM agent_run_logs
		WHERE tenant_id = $1 AND status = 'success' AND created_at >= now() - MAKE_INTERVAL(DAYS => $2)
	`, tenantID, days).Scan(&current.AgentIndependentMs, &current.ReworkTotal, &current.TotalRuns); err != nil {
		return nil, err
	}

	if err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_duration_ms), 0)::int8,
		       COALESCE(SUM(active_duration_ms), 0)::int8,
		       COUNT(*)::int
		FROM conversations
		WHERE tenant_id = $1 AND created_at >= now() - MAKE_INTERVAL(DAYS => $2)
	`, tenantID, days).Scan(&current.ConversationTotalMs, &current.ConversationActiveMs, &current.TotalConversations); err != nil {
		return nil, err
	}

	if err := r.db.QueryRow(ctx, `
		SELECT COALESCE(AVG(duration_ms), 0)::int8
		FROM approval_tasks
		WHERE tenant_id = $1 AND duration_ms IS NOT NULL AND created_at >= now() - MAKE_INTERVAL(DAYS => $2)
	`, tenantID, days).Scan(&current.ApprovalAvgMs); err != nil {
		return nil, err
	}

	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM approval_tasks
		WHERE tenant_id = $1 AND created_at >= now() - MAKE_INTERVAL(DAYS => $2)
	`, tenantID, days).Scan(&current.TotalApprovals); err != nil {
		return nil, err
	}

	for date, metrics := range response.MetricsByDate {
		metrics.ConversationTotalMs = current.ConversationTotalMs
		metrics.ConversationActiveMs = current.ConversationActiveMs
		metrics.ApprovalAvgMs = current.ApprovalAvgMs
		metrics.TotalConversations = current.TotalConversations
		metrics.TotalApprovals = current.TotalApprovals
		response.MetricsByDate[date] = metrics
	}

	response.Current = current
	return response, nil
}

// allowedTables whitelist for countBuckets to prevent SQL injection
var allowedTables = map[string]bool{
	"workflow_instances": true,
	"agent_run_logs":     true,
}

// allowedColumns whitelist for countBuckets
var allowedColumns = map[string]bool{
	"status": true,
}

func (r *Repository) countBuckets(ctx context.Context, table, column, where, tenantID string) ([]CountBucket, error) {
	if !allowedTables[table] {
		return nil, fmt.Errorf("countBuckets: table %q is not whitelisted", table)
	}
	if !allowedColumns[column] {
		return nil, fmt.Errorf("countBuckets: column %q is not whitelisted", column)
	}
	if strings.Contains(where, "--") || strings.Contains(strings.ToLower(where), "drop ") || strings.Contains(strings.ToLower(where), "union ") {
		return nil, fmt.Errorf("countBuckets: suspicious SQL in where clause")
	}

	query := fmt.Sprintf(`SELECT %s, COUNT(*) FROM %s WHERE %s GROUP BY %s ORDER BY COUNT(*) DESC, %s`, column, table, where, column, column)
	rows, err := r.db.Query(ctx, query, tenantID)
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
