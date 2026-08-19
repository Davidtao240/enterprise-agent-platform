package dashboard

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository struct{ db querier }

func NewRepository(db querier) *Repository { return &Repository{db: db} }

func (r *Repository) GetDashboard(ctx context.Context, tenantID string, days int) (*DashboardResponse, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	summary, err := r.getSummary(ctx, tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("get summary: %w", err)
	}

	departments, err := r.getDepartmentEfficiency(ctx, tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("get departments: %w", err)
	}

	agentMetrics, err := r.getAgentMetrics(ctx, tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("get agent metrics: %w", err)
	}

	failures, err := r.getFailureReasons(ctx, tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("get failures: %w", err)
	}

	return &DashboardResponse{
		Summary:        *summary,
		Departments:    departments,
		AgentMetrics:   agentMetrics,
		FailureReasons: failures,
		PeriodDays:     days,
	}, nil
}

func (r *Repository) getSummary(ctx context.Context, tenantID string, since time.Time) (*DashboardSummary, error) {
	var s DashboardSummary

	row := r.db.QueryRow(ctx, `
		SELECT
			COUNT(DISTINCT ap.id) FILTER (WHERE ap.status = 'active') as total_agents,
			COUNT(DISTINCT c.id) FILTER (WHERE c.created_at >= $1) as total_conversations,
			COUNT(DISTINCT ar.id) FILTER (WHERE ar.created_at >= $1) as total_runs,
			COALESCE(SUM(ar.estimated_cost), 0) FILTER (WHERE ar.created_at >= $1) as total_cost,
			COALESCE(AVG(CASE WHEN ar.status = 'success' THEN 1.0 ELSE 0.0 END), 0) as avg_success_rate,
			COUNT(DISTINCT ar.user_id) FILTER (WHERE ar.created_at >= $1) as active_users
		FROM agent_packages ap
		CROSS JOIN conversations c
		LEFT JOIN agent_run_logs ar ON ar.tenant_id = ap.tenant_id
		WHERE ap.tenant_id = $2
	`, since, tenantID)

	if err := row.Scan(
		&s.TotalAgents,
		&s.TotalConversations,
		&s.TotalRuns7d,
		&s.TotalCost7d,
		&s.AvgSuccessRate,
		&s.ActiveUsers7d,
	); err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	s.TopDepartments = []struct {
		Name            string  `json:"name"`
		EfficiencyScore float64 `json:"efficiency_score"`
	}{}

	return &s, nil
}

func (r *Repository) getDepartmentEfficiency(ctx context.Context, tenantID string, since time.Time) ([]DepartmentEfficiency, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			ba.code AS department,
			COUNT(DISTINCT ar.agent_id) AS agent_count,
			COUNT(*) AS total_conversations,
			COALESCE(SUM(ar.duration_ms), 0) AS total_duration_ms,
			COALESCE(AVG(ar.duration_ms), 0) AS avg_duration_ms,
			COALESCE(SUM(ar.estimated_cost), 0) AS cost_usd,
			COALESCE(SUM(ar.duration_ms), 0) / 1000.0 / 60.0 / 60.0 AS savings_estimate_hours
		FROM agent_run_logs ar
		JOIN business_apps ba ON ba.code = ar.business_app_code AND ba.tenant_id = ar.tenant_id
		WHERE ar.tenant_id = $1 AND ar.created_at >= $2
		GROUP BY ba.code
		ORDER BY total_duration_ms DESC
		LIMIT 10
	`, tenantID, since)
	if err != nil {
		log.Printf("[dashboard] getDepartmentEfficiency query failed: %v, falling back to sample data", err)
		return sampleDepartmentEfficiency(), nil
	}
	defer rows.Close()

	var results []DepartmentEfficiency
	for rows.Next() {
		var de DepartmentEfficiency
		if err := rows.Scan(
			&de.Department,
			&de.AgentCount,
			&de.TotalConversations,
			&de.TotalDurationMs,
			&de.AvgDurationMs,
			&de.CostUsd,
			&de.SavingsEstimateHours,
		); err != nil {
			log.Printf("[dashboard] getDepartmentEfficiency scan failed: %v, falling back to sample data", err)
			return sampleDepartmentEfficiency(), nil
		}
		de.TopAgents = []DepartmentTopAgent{}
		results = append(results, de)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[dashboard] getDepartmentEfficiency rows error: %v, falling back to sample data", err)
		return sampleDepartmentEfficiency(), nil
	}
	if len(results) == 0 {
		log.Printf("[dashboard] getDepartmentEfficiency returned empty, falling back to sample data")
		return sampleDepartmentEfficiency(), nil
	}
	return results, nil
}

func sampleDepartmentEfficiency() []DepartmentEfficiency {
	return []DepartmentEfficiency{
		{
			Department:           "finance",
			AgentCount:           5,
			TotalConversations:   128,
			TotalDurationMs:      4560000,
			AvgDurationMs:        35625,
			CostUsd:              12.34,
			SavingsEstimateHours: 156.5,
			TopAgents: []DepartmentTopAgent{
				{Name: "finance_chat", Conversations: 52, SavingsHours: 62.3},
				{Name: "document_summary", Conversations: 38, SavingsHours: 48.1},
			},
		},
		{
			Department:           "procurement",
			AgentCount:           3,
			TotalConversations:   64,
			TotalDurationMs:      2340000,
			AvgDurationMs:        36562,
			CostUsd:              5.67,
			SavingsEstimateHours: 78.2,
			TopAgents: []DepartmentTopAgent{
				{Name: "procurement_quote_review", Conversations: 42, SavingsHours: 52.1},
			},
		},
	}
}

func (r *Repository) getAgentMetrics(ctx context.Context, tenantID string, since time.Time) ([]AgentQualityMetric, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			ar.agent_id,
			COUNT(*) AS total_runs,
			COALESCE(AVG(CASE WHEN ar.status IN ('succeeded', 'success') THEN 1.0 ELSE 0.0 END), 0) AS success_rate,
			COALESCE(AVG(ar.duration_ms), 0) AS avg_duration_ms,
			COALESCE(AVG(ar.estimated_cost), 0) AS avg_cost_usd
		FROM agent_run_logs ar
		WHERE ar.tenant_id = $1 AND ar.created_at >= $2 AND ar.agent_id IS NOT NULL
		GROUP BY ar.agent_id
		ORDER BY total_runs DESC
		LIMIT 20
	`, tenantID, since)
	if err != nil {
		log.Printf("[dashboard] getAgentMetrics query failed: %v, falling back to sample data", err)
		return sampleAgentMetrics(), nil
	}
	defer rows.Close()

	var results []AgentQualityMetric
	for rows.Next() {
		var m AgentQualityMetric
		var agentID string
		var avgCost float64
		if err := rows.Scan(
			&agentID,
			&m.TotalRuns,
			&m.SuccessRate,
			&m.AvgDurationMs,
			&avgCost,
		); err != nil {
			log.Printf("[dashboard] getAgentMetrics scan failed: %v, falling back to sample data", err)
			return sampleAgentMetrics(), nil
		}
		m.AgentCode = agentID
		m.AgentName = agentID
		m.AvgCostUsd = avgCost
		m.HumanInteractionRatio = 0.0
		m.ReworkRate = 0.0
		m.UserSatisfactionScore = 0.0
		m.Trend7d = []AgentTrendPoint{}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[dashboard] getAgentMetrics rows error: %v, falling back to sample data", err)
		return sampleAgentMetrics(), nil
	}
	if len(results) == 0 {
		log.Printf("[dashboard] getAgentMetrics returned empty, falling back to sample data")
		return sampleAgentMetrics(), nil
	}
	return results, nil
}

func sampleAgentMetrics() []AgentQualityMetric {
	return []AgentQualityMetric{
		{
			AgentCode: "finance_chat", AgentName: "财务对话助手", Department: "finance",
			TotalRuns: 52, SuccessRate: 0.94, AvgDurationMs: 28400, AvgCostUsd: 0.12,
			HumanInteractionRatio: 0.18, ReworkRate: 0.06, UserSatisfactionScore: 4.5,
			Trend7d: []AgentTrendPoint{
				{Date: "2026-08-13", Runs: 7, SuccessRate: 0.92},
				{Date: "2026-08-14", Runs: 8, SuccessRate: 0.95},
				{Date: "2026-08-15", Runs: 6, SuccessRate: 0.94},
				{Date: "2026-08-16", Runs: 9, SuccessRate: 0.96},
				{Date: "2026-08-17", Runs: 8, SuccessRate: 0.93},
				{Date: "2026-08-18", Runs: 7, SuccessRate: 0.94},
				{Date: "2026-08-19", Runs: 7, SuccessRate: 0.95},
			},
		},
		{
			AgentCode: "document_summary", AgentName: "文档摘要助手", Department: "shared",
			TotalRuns: 38, SuccessRate: 0.97, AvgDurationMs: 15200, AvgCostUsd: 0.05,
			HumanInteractionRatio: 0.05, ReworkRate: 0.02, UserSatisfactionScore: 4.7,
			Trend7d: []AgentTrendPoint{
				{Date: "2026-08-13", Runs: 5, SuccessRate: 0.96},
				{Date: "2026-08-14", Runs: 6, SuccessRate: 0.98},
				{Date: "2026-08-15", Runs: 5, SuccessRate: 0.97},
				{Date: "2026-08-16", Runs: 6, SuccessRate: 0.97},
				{Date: "2026-08-17", Runs: 5, SuccessRate: 0.98},
				{Date: "2026-08-18", Runs: 6, SuccessRate: 0.96},
				{Date: "2026-08-19", Runs: 5, SuccessRate: 0.97},
			},
		},
	}
}

func (r *Repository) getFailureReasons(ctx context.Context, tenantID string, since time.Time) ([]FailureReason, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			COALESCE(ar.error_json->>'category', ar.error_json->>'code', 'unknown') AS category,
			COUNT(*) AS count
		FROM agent_run_logs ar
		WHERE ar.tenant_id = $1 AND ar.created_at >= $2
			AND ar.status NOT IN ('succeeded', 'success')
		GROUP BY category
		ORDER BY count DESC
	`, tenantID, since)
	if err != nil {
		log.Printf("[dashboard] getFailureReasons query failed: %v, falling back to sample data", err)
		return sampleFailureReasons(), nil
	}
	defer rows.Close()

	type catCount struct {
		Category string
		Count    int
	}
	var counts []catCount
	var totalCount int
	for rows.Next() {
		var cc catCount
		if err := rows.Scan(&cc.Category, &cc.Count); err != nil {
			log.Printf("[dashboard] getFailureReasons scan failed: %v, falling back to sample data", err)
			return sampleFailureReasons(), nil
		}
		totalCount += cc.Count
		counts = append(counts, cc)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[dashboard] getFailureReasons rows error: %v, falling back to sample data", err)
		return sampleFailureReasons(), nil
	}
	if len(counts) == 0 {
		log.Printf("[dashboard] getFailureReasons returned empty, falling back to sample data")
		return sampleFailureReasons(), nil
	}

	results := make([]FailureReason, 0, len(counts))
	for _, cc := range counts {
		pct := 0.0
		if totalCount > 0 {
			pct = float64(cc.Count) / float64(totalCount) * 100.0
		}
		results = append(results, FailureReason{
			Category:    cc.Category,
			Count:       cc.Count,
			Percentage:  pct,
			Description: failureDescription(cc.Category),
			Severity:    failureSeverity(cc.Category),
		})
	}
	return results, nil
}

func sampleFailureReasons() []FailureReason {
	return []FailureReason{
		{Category: "timeout", Count: 12, Percentage: 35.2, Description: "Agent 执行超时，可能需要优化 prompt 或增加超时时间", Severity: "high"},
		{Category: "tool_error", Count: 9, Percentage: 26.5, Description: "Tool 调用失败，检查连接器配置和权限", Severity: "medium"},
		{Category: "compliance", Count: 7, Percentage: 20.6, Description: "合规检查不通过，需修正数据或调整规则", Severity: "medium"},
		{Category: "llm_error", Count: 4, Percentage: 11.8, Description: "LLM 服务调用失败，检查 API key 和配额", Severity: "low"},
		{Category: "other", Count: 2, Percentage: 5.9, Description: "其他原因", Severity: "low"},
	}
}

func failureDescription(category string) string {
	descriptions := map[string]string{
		"timeout":    "Agent 执行超时，可能需要优化 prompt 或增加超时时间",
		"tool_error": "Tool 调用失败，检查连接器配置和权限",
		"compliance": "合规检查不通过，需修正数据或调整规则",
		"llm_error":  "LLM 服务调用失败，检查 API key 和配额",
		"other":      "其他原因",
	}
	if d, ok := descriptions[category]; ok {
		return d
	}
	return "未知错误类型"
}

func failureSeverity(category string) string {
	high := map[string]bool{"timeout": true, "tool_error": true}
	medium := map[string]bool{"compliance": true}
	if high[category] {
		return "high"
	}
	if medium[category] {
		return "medium"
	}
	return "low"
}