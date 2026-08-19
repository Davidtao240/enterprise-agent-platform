package dashboard

type DashboardSummary struct {
	TotalAgents       int     `json:"total_agents"`
	TotalConversations int    `json:"total_conversations"`
	TotalRuns7d       int     `json:"total_runs_7d"`
	TotalCost7d       float64 `json:"total_cost_7d"`
	AvgSuccessRate    float64 `json:"avg_success_rate"`
	ActiveUsers7d     int     `json:"active_users_7d"`
	TopDepartments    []struct {
		Name            string  `json:"name"`
		EfficiencyScore float64 `json:"efficiency_score"`
	} `json:"top_departments"`
}

type DepartmentEfficiency struct {
	Department          string               `json:"department"`
	AgentCount          int                  `json:"agent_count"`
	TotalConversations  int                  `json:"total_conversations"`
	TotalDurationMs     int64                `json:"total_duration_ms"`
	AvgDurationMs       int64                `json:"avg_duration_ms"`
	CostUsd             float64              `json:"cost_usd"`
	SavingsEstimateHours float64             `json:"savings_estimate_hours"`
	TopAgents           []DepartmentTopAgent `json:"top_agents"`
}

type DepartmentTopAgent struct {
	Name        string  `json:"name"`
	Conversations int    `json:"conversations"`
	SavingsHours float64 `json:"savings_hours"`
}

type AgentQualityMetric struct {
	AgentCode             string            `json:"agent_code"`
	AgentName             string            `json:"agent_name"`
	Department            string            `json:"department"`
	TotalRuns             int               `json:"total_runs"`
	SuccessRate           float64           `json:"success_rate"`
	AvgDurationMs         int64             `json:"avg_duration_ms"`
	AvgCostUsd            float64           `json:"avg_cost_usd"`
	HumanInteractionRatio float64           `json:"human_interaction_ratio"`
	ReworkRate            float64           `json:"rework_rate"`
	UserSatisfactionScore float64           `json:"user_satisfaction_score"`
	Trend7d               []AgentTrendPoint `json:"trend_7d"`
}

type AgentTrendPoint struct {
	Date        string  `json:"date"`
	Runs        int     `json:"runs"`
	SuccessRate float64 `json:"success_rate"`
}

type FailureReason struct {
	Category    string  `json:"category"`
	Count       int     `json:"count"`
	Percentage  float64 `json:"percentage"`
	Description string  `json:"description"`
	Severity    string  `json:"severity"`
}

type DashboardResponse struct {
	Summary        DashboardSummary       `json:"summary"`
	Departments    []DepartmentEfficiency  `json:"departments"`
	AgentMetrics   []AgentQualityMetric   `json:"agent_metrics"`
	FailureReasons []FailureReason        `json:"failure_reasons"`
	PeriodDays     int                    `json:"period_days"`
}