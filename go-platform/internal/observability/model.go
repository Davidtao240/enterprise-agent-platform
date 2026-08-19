package observability

type CountBucket struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type Summary struct {
	WorkflowByStatus  []CountBucket `json:"workflow_by_status"`
	AgentRunByStatus  []CountBucket `json:"agent_run_by_status"`
	FailedRunsLast24h int           `json:"failed_runs_last_24h"`
	AverageDurationMs int           `json:"average_duration_ms"`
	TotalReportedCost float64       `json:"total_reported_cost"`
}

type Alert struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type SummaryResponse struct {
	Summary Summary `json:"summary"`
	Alerts  []Alert `json:"alerts"`
}

type AVRMetrics struct {
	AgentIndependentMs   int64  `json:"agent_independent_ms"`
	ConversationTotalMs  int64  `json:"conversation_total_ms"`
	ConversationActiveMs int64  `json:"conversation_active_ms"`
	ApprovalAvgMs        int64  `json:"approval_avg_ms"`
	ReworkTotal          int    `json:"rework_total"`
	TotalRuns            int    `json:"total_runs"`
	TotalConversations   int    `json:"total_conversations"`
	TotalApprovals       int    `json:"total_approvals"`
}

type AVRResponse struct {
	MetricsByDate map[string]AVRMetrics `json:"metrics_by_date"`
	Current       AVRMetrics            `json:"current"`
}
