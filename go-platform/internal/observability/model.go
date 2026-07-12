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
