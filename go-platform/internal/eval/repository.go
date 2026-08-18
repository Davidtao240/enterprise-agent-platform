package eval

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ── 窗口聚合结构(原始计数,Service 层换算比率) ──

// RunStats agent_run_logs 窗口内终态 Run 聚合(已排除 shadow/replay Run)。
type RunStats struct {
	TotalRuns       float64 // succeeded + failed + cancelled
	FailedRuns      float64
	TotalTokens     float64
	TotalCost       float64
	TotalDurationMs float64
}

// LLMStats trace_events L3 聚合。
type LLMStats struct {
	TurnEnds       float64
	LatencyTotalMs float64
}

// ToolStats tool_calls 窗口内终态调用聚合。
type ToolStats struct {
	Succeeded     float64
	Failed        float64
	Indeterminate float64
}

// ApprovalStats approval_tasks 窗口内已决策审批聚合(经 workflow_instances 租户过滤)。
type ApprovalStats struct {
	Approved float64
	Rejected float64
}

// MetricsSource 指标数据源(由 Repository 实现;测试用替身)。
type MetricsSource interface {
	RunStats(ctx context.Context, tenantID string, start, end time.Time, filters map[string]string) (RunStats, error)
	LLMStats(ctx context.Context, tenantID string, start, end time.Time) (LLMStats, error)
	ToolStats(ctx context.Context, tenantID string, start, end time.Time) (ToolStats, error)
	ApprovalStats(ctx context.Context, tenantID string, start, end time.Time) (ApprovalStats, error)
	InterruptCount(ctx context.Context, tenantID string, start, end time.Time) (int64, error)
}

// Repository pgx 实现的 MetricsSource。
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository 创建 Repository。
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// runFiltersToSQL 将白名单过滤条件拼入 SQL(参数化,值以 $n 占位)。
// 支持 agent_id / workflow_id / graph_key (Spec §3.2 Filters)。
func runFiltersToSQL(filters map[string]string, nextArg int, args *[]any) (string, int) {
	clause := ""
	add := func(col, val string) {
		*args = append(*args, val)
		clause += " AND arl." + col + " = $" + strconv.Itoa(nextArg)
		nextArg++
	}
	if v, ok := filters["agent_id"]; ok && v != "" {
		add("agent_id", v)
	}
	if v, ok := filters["workflow_id"]; ok && v != "" {
		add("workflow_instance_id", v)
	}
	if v, ok := filters["graph_key"]; ok && v != "" {
		add("graph_key", v)
	}
	return clause, nextArg
}

// RunStats 聚合窗口内终态 Run(排除规则见 TRACE_AND_EVAL.md §3.3:
// LEFT JOIN agent_runs 按 metadata_json 排除 shadow/replay 标记)。
func (r *Repository) RunStats(ctx context.Context, tenantID string, start, end time.Time, filters map[string]string) (RunStats, error) {
	var out RunStats
	args := []any{tenantID, start, end}
	filterClause, _ := runFiltersToSQL(filters, 4, &args)
	query := `SELECT
		COUNT(*),
		COUNT(*) FILTER (WHERE arl.status = 'failed'),
		COALESCE(SUM((arl.usage_json->>'total_tokens')::numeric), 0),
		COALESCE(SUM((arl.usage_json->>'cost')::numeric), 0),
		COALESCE(SUM(arl.duration_ms), 0)
	FROM agent_run_logs arl
	LEFT JOIN agent_runs ar ON ar.id = arl.durable_run_id
	WHERE arl.tenant_id = $1
	  AND arl.status IN ('succeeded','failed','cancelled')
	  AND arl.finished_at >= $2 AND arl.finished_at < $3
	  AND (ar.id IS NULL OR COALESCE(ar.metadata_json->>'shadow', 'false') <> 'true')
	  AND (ar.id IS NULL OR COALESCE(ar.metadata_json->>'replay', 'false') <> 'true')` + filterClause
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&out.TotalRuns, &out.FailedRuns, &out.TotalTokens, &out.TotalCost, &out.TotalDurationMs,
	)
	return out, err
}

// LLMStats 聚合窗口内 L3 Model Turn end 事件耗时。
func (r *Repository) LLMStats(ctx context.Context, tenantID string, start, end time.Time) (LLMStats, error) {
	var out LLMStats
	err := r.pool.QueryRow(ctx, `SELECT
		COUNT(*), COALESCE(SUM(duration_ms), 0)
		FROM trace_events
		WHERE tenant_id = $1 AND layer = 'L3' AND event_type = 'end'
		  AND timestamp >= $2 AND timestamp < $3
		  AND duration_ms IS NOT NULL`,
		tenantID, start, end,
	).Scan(&out.TurnEnds, &out.LatencyTotalMs)
	return out, err
}

// ToolStats 聚合窗口内终态 Tool Call。
func (r *Repository) ToolStats(ctx context.Context, tenantID string, start, end time.Time) (ToolStats, error) {
	var out ToolStats
	err := r.pool.QueryRow(ctx, `SELECT
		COUNT(*) FILTER (WHERE status = 'succeeded'),
		COUNT(*) FILTER (WHERE status = 'failed'),
		COUNT(*) FILTER (WHERE status = 'indeterminate')
		FROM tool_calls
		WHERE tenant_id = $1
		  AND status IN ('succeeded','failed','indeterminate')
		  AND created_at >= $2 AND created_at < $3`,
		tenantID, start, end,
	).Scan(&out.Succeeded, &out.Failed, &out.Indeterminate)
	return out, err
}

// ApprovalStats 聚合窗口内已决策审批(approval_tasks 经 workflow_instances 关联租户)。
func (r *Repository) ApprovalStats(ctx context.Context, tenantID string, start, end time.Time) (ApprovalStats, error) {
	var out ApprovalStats
	err := r.pool.QueryRow(ctx, `SELECT
		COUNT(*) FILTER (WHERE at.status = 'approved'),
		COUNT(*) FILTER (WHERE at.status = 'rejected')
		FROM approval_tasks at
		JOIN workflow_instances wi ON wi.id = at.workflow_instance_id
		WHERE wi.tenant_id = $1
		  AND at.decided_at >= $2 AND at.decided_at < $3`,
		tenantID, start, end,
	).Scan(&out.Approved, &out.Rejected)
	return out, err
}

// InterruptCount 统计窗口内 L6 Interrupt 事件数。
func (r *Repository) InterruptCount(ctx context.Context, tenantID string, start, end time.Time) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*)
		FROM trace_events
		WHERE tenant_id = $1 AND layer = 'L6' AND event_type = 'interrupted'
		  AND timestamp >= $2 AND timestamp < $3`,
		tenantID, start, end,
	).Scan(&count)
	return count, err
}
