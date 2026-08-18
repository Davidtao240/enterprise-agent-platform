package eval

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// trendTolerance 相对变化小于 1% 视为 stable (Spec §3.3)。
const trendTolerance = 0.01

// Service 评估报告生成器:当前窗口 + 等长上一窗口各取一次聚合,换算指标并对比。
type Service struct {
	source MetricsSource
}

// NewService 创建 Service。
func NewService(source MetricsSource) *Service { return &Service{source: source} }

// allowedFilters Filters 白名单(其余键拒绝,避免开放拼接)。
var allowedFilters = map[string]bool{"agent_id": true, "workflow_id": true, "graph_key": true}

// GenerateReport 计算窗口指标并与上一周期对比。
func (s *Service) GenerateReport(ctx context.Context, tenantID string, req ReportRequest) (*ReportResponse, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if req.StartTime.IsZero() || req.EndTime.IsZero() {
		return nil, errors.New("start_time and end_time are required")
	}
	if !req.StartTime.Before(req.EndTime) {
		return nil, errors.New("start_time must be before end_time")
	}
	for k := range req.Filters {
		if !allowedFilters[k] {
			return nil, fmt.Errorf("unsupported filter key %q (expect agent_id/workflow_id/graph_key)", k)
		}
	}
	metrics := req.Metrics
	if len(metrics) == 0 {
		metrics = AllMetrics
	}
	for _, m := range metrics {
		if !IsKnownMetric(m) {
			return nil, fmt.Errorf("unknown metric %q", m)
		}
	}

	windowLen := req.EndTime.Sub(req.StartTime)
	prevEnd := req.StartTime
	prevStart := prevEnd.Add(-windowLen)

	cur, err := s.collect(ctx, tenantID, req.StartTime, req.EndTime, req.Filters)
	if err != nil {
		return nil, err
	}
	prev, err := s.collect(ctx, tenantID, prevStart, prevEnd, req.Filters)
	if err != nil {
		return nil, err
	}

	// 窗口内全部指标值(便于 Summary 呈现),Details 只输出请求的指标。
	curValues := computeValues(cur)
	prevValues := computeValues(prev)

	resp := &ReportResponse{
		Window: ReportWindow{
			Start:         req.StartTime,
			End:           req.EndTime,
			PreviousStart: prevStart,
			PreviousEnd:   prevEnd,
		},
		Summary: map[string]float64{},
	}
	seen := map[string]bool{}
	for _, m := range metrics {
		if seen[m] {
			continue
		}
		seen[m] = true
		resp.Summary[m] = curValues[m]
		resp.Details = append(resp.Details, MetricDetail{
			MetricName:    m,
			Value:         curValues[m],
			PreviousValue: prevValues[m],
			Trend:         trend(curValues[m], prevValues[m]),
			Comparison:    comparison(curValues[m], prevValues[m]),
		})
	}
	return resp, nil
}

// windowAgg 一个窗口的全部原始聚合。
type windowAgg struct {
	Run       RunStats
	LLM       LLMStats
	Tool      ToolStats
	Approval  ApprovalStats
	Interrupt float64
}

func (s *Service) collect(ctx context.Context, tenantID string, start, end time.Time, filters map[string]string) (windowAgg, error) {
	var agg windowAgg
	var err error
	if agg.Run, err = s.source.RunStats(ctx, tenantID, start, end, filters); err != nil {
		return agg, fmt.Errorf("run stats: %w", err)
	}
	if agg.LLM, err = s.source.LLMStats(ctx, tenantID, start, end); err != nil {
		return agg, fmt.Errorf("llm stats: %w", err)
	}
	if agg.Tool, err = s.source.ToolStats(ctx, tenantID, start, end); err != nil {
		return agg, fmt.Errorf("tool stats: %w", err)
	}
	if agg.Approval, err = s.source.ApprovalStats(ctx, tenantID, start, end); err != nil {
		return agg, fmt.Errorf("approval stats: %w", err)
	}
	n, err := s.source.InterruptCount(ctx, tenantID, start, end)
	if err != nil {
		return agg, fmt.Errorf("interrupt count: %w", err)
	}
	agg.Interrupt = float64(n)
	return agg, nil
}

// computeValues 从原始聚合换算九项指标(比率类分母为 0 时记 0)。
func computeValues(a windowAgg) map[string]float64 {
	toolTerminal := a.Tool.Succeeded + a.Tool.Failed + a.Tool.Indeterminate
	approvalDecided := a.Approval.Approved + a.Approval.Rejected
	return map[string]float64{
		MetricTotalTokens:      a.Run.TotalTokens,
		MetricTotalCost:        roundTo(a.Run.TotalCost, 6),
		MetricTotalDuration:    a.Run.TotalDurationMs,
		MetricAvgDuration:      div(a.Run.TotalDurationMs, a.Run.TotalRuns),
		MetricLLMLatency:       div(a.LLM.LatencyTotalMs, a.LLM.TurnEnds),
		MetricToolSuccessRate:  div(a.Tool.Succeeded, toolTerminal),
		MetricApprovalPassRate: div(a.Approval.Approved, approvalDecided),
		MetricErrorRate:        div(a.Run.FailedRuns, a.Run.TotalRuns),
		MetricInterruptCount:   a.Interrupt,
	}
}

func div(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// trend 相对变化判定:up / down / stable(<1%)。prev=0 且 cur>0 视为 up。
func trend(cur, prev float64) string {
	rel := relative(cur, prev)
	switch {
	case rel > trendTolerance:
		return "up"
	case rel < -trendTolerance:
		return "down"
	default:
		return "stable"
	}
}

// comparison 相对变化百分比 (cur-prev)/prev*100,prev=0 时为 0 (Spec §3.3)。
func comparison(cur, prev float64) float64 {
	if prev == 0 {
		return 0
	}
	return roundTo((cur-prev)/prev*100, 2)
}

func relative(cur, prev float64) float64 {
	if prev == 0 {
		if cur == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return (cur - prev) / prev
}

func roundTo(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}
