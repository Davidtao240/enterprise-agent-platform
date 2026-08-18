// Package eval 实现 M5-B 评估体系:基于 Trace/Run/ToolCall/Approval 数据
// 按时间窗口计算成本、效率、质量、稳定性指标,并与等长上一窗口对比。
//
// Spec: docs/03_PLATFORM_SPEC/TRACE_AND_EVAL.md §3
package eval

import "time"

// 指标名 (Spec §3.1/§3.3)。
const (
	MetricTotalTokens      = "total_tokens"
	MetricTotalCost        = "total_cost"
	MetricTotalDuration    = "total_duration"
	MetricAvgDuration      = "avg_duration"
	MetricLLMLatency       = "llm_latency"
	MetricToolSuccessRate  = "tool_success_rate"
	MetricApprovalPassRate = "approval_pass_rate"
	MetricErrorRate        = "error_rate"
	MetricInterruptCount   = "interrupt_count"
)

// AllMetrics 全部可计算指标(请求 Metrics 为空时默认计算)。
var AllMetrics = []string{
	MetricTotalTokens, MetricTotalCost, MetricTotalDuration, MetricAvgDuration,
	MetricLLMLatency, MetricToolSuccessRate, MetricApprovalPassRate,
	MetricErrorRate, MetricInterruptCount,
}

func IsKnownMetric(name string) bool {
	for _, m := range AllMetrics {
		if m == name {
			return true
		}
	}
	return false
}

// ReportRequest 评估报告请求 (Spec §3.2)。
type ReportRequest struct {
	// StartTime / EndTime 当前统计窗口 [Start, End)。必填且 Start < End。
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	// Filters 过滤条件,支持 agent_id / workflow_id / graph_key。
	Filters map[string]string `json:"filters"`
	// Metrics 要计算的指标列表;空表示全部。
	Metrics []string `json:"metrics"`
}

// ReportWindow 当前与上一对比窗口信息。
type ReportWindow struct {
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	PreviousStart time.Time `json:"previous_start"`
	PreviousEnd   time.Time `json:"previous_end"`
}

// MetricDetail 单指标详情 (Spec §3.2)。
type MetricDetail struct {
	MetricName    string  `json:"metric_name"`
	Value         float64 `json:"value"`
	Trend         string  `json:"trend"`      // 'up' | 'down' | 'stable'
	Comparison    float64 `json:"comparison"` // 与上一窗口的相对变化百分比
	PreviousValue float64 `json:"previous_value"`
}

// ReportResponse 评估报告响应 (Spec §3.2)。
type ReportResponse struct {
	Window  ReportWindow      `json:"window"`
	Summary map[string]float64 `json:"summary"`
	Details []MetricDetail    `json:"details"`
}
