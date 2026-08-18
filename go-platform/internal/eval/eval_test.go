package eval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// fakeSource 可编程的 MetricsSource 替身:当前窗口 [baseTime, +24h) 返回 cur,
// 上一窗口 [baseTime-24h, baseTime) 返回 prev。
type fakeSource struct {
	cur, prev windowAgg
}

func (f *fakeSource) isCurrent(start time.Time) bool { return !start.Before(baseTime) }

func (f *fakeSource) RunStats(_ context.Context, _ string, start, _ time.Time, _ map[string]string) (RunStats, error) {
	if f.isCurrent(start) {
		return f.cur.Run, nil
	}
	return f.prev.Run, nil
}
func (f *fakeSource) LLMStats(_ context.Context, _ string, start, _ time.Time) (LLMStats, error) {
	if f.isCurrent(start) {
		return f.cur.LLM, nil
	}
	return f.prev.LLM, nil
}
func (f *fakeSource) ToolStats(_ context.Context, _ string, start, _ time.Time) (ToolStats, error) {
	if f.isCurrent(start) {
		return f.cur.Tool, nil
	}
	return f.prev.Tool, nil
}
func (f *fakeSource) ApprovalStats(_ context.Context, _ string, start, _ time.Time) (ApprovalStats, error) {
	if f.isCurrent(start) {
		return f.cur.Approval, nil
	}
	return f.prev.Approval, nil
}
func (f *fakeSource) InterruptCount(_ context.Context, _ string, start, _ time.Time) (int64, error) {
	if f.isCurrent(start) {
		return int64(f.cur.Interrupt), nil
	}
	return int64(f.prev.Interrupt), nil
}

var baseTime = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

func testWindow() (time.Time, time.Time) {
	return baseTime, baseTime.Add(24 * time.Hour)
}

func TestGenerateReportAllMetrics(t *testing.T) {
	src := &fakeSource{
		cur: windowAgg{
			Run:       RunStats{TotalRuns: 10, FailedRuns: 2, TotalTokens: 5000, TotalCost: 1.25, TotalDurationMs: 100000},
			LLM:       LLMStats{TurnEnds: 50, LatencyTotalMs: 25000},
			Tool:      ToolStats{Succeeded: 45, Failed: 5, Indeterminate: 0},
			Approval:  ApprovalStats{Approved: 8, Rejected: 2},
			Interrupt: 3,
		},
		prev: windowAgg{
			Run:       RunStats{TotalRuns: 20, FailedRuns: 2, TotalTokens: 4000, TotalCost: 1.0, TotalDurationMs: 300000},
			LLM:       LLMStats{TurnEnds: 40, LatencyTotalMs: 40000},
			Tool:      ToolStats{Succeeded: 30, Failed: 10, Indeterminate: 0},
			Approval:  ApprovalStats{Approved: 9, Rejected: 1},
			Interrupt: 3,
		},
	}
	svc := NewService(src)
	start, end := testWindow()
	resp, err := svc.GenerateReport(context.Background(), "tenant-1", ReportRequest{StartTime: start, EndTime: end})
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	expect := map[string]float64{
		MetricTotalTokens:      5000,
		MetricTotalCost:        1.25,
		MetricTotalDuration:    100000,
		MetricAvgDuration:      10000, // 100000/10
		MetricLLMLatency:       500,   // 25000/50
		MetricToolSuccessRate:  0.9,   // 45/50
		MetricApprovalPassRate: 0.8,   // 8/10
		MetricErrorRate:        0.2,   // 2/10
		MetricInterruptCount:   3,
	}
	if len(resp.Summary) != len(expect) {
		t.Fatalf("expected %d summary metrics, got %d", len(expect), len(resp.Summary))
	}
	for name, want := range expect {
		if got := resp.Summary[name]; got != want {
			t.Errorf("metric %s = %v, want %v", name, got, want)
		}
	}

	// Trend / Comparison 抽查:tokens 上涨 25%,error_rate 从 0.1 → 0.2 上涨 100%,interrupt stable。
	details := map[string]MetricDetail{}
	for _, d := range resp.Details {
		details[d.MetricName] = d
	}
	if d := details[MetricTotalTokens]; d.Trend != "up" || d.Comparison != 25 {
		t.Errorf("total_tokens detail = %+v, want up/25", d)
	}
	if d := details[MetricErrorRate]; d.Trend != "up" || d.Comparison != 100 {
		t.Errorf("error_rate detail = %+v, want up/100", d)
	}
	if d := details[MetricInterruptCount]; d.Trend != "stable" || d.Comparison != 0 {
		t.Errorf("interrupt_count detail = %+v, want stable/0", d)
	}
	if d := details[MetricToolSuccessRate]; d.Trend != "up" {
		t.Errorf("tool_success_rate trend = %q, want up", d.Trend)
	}

	// 上一窗口边界:[start-24h, start)。
	if !resp.Window.PreviousEnd.Equal(start) {
		t.Errorf("previous window end = %v, want %v", resp.Window.PreviousEnd, start)
	}
	if resp.Window.PreviousEnd.Sub(resp.Window.PreviousStart) != end.Sub(start) {
		t.Error("previous window length mismatch")
	}
}

func TestGenerateReportZeroDenominators(t *testing.T) {
	svc := NewService(&fakeSource{})
	start, end := testWindow()
	resp, err := svc.GenerateReport(context.Background(), "tenant-1", ReportRequest{StartTime: start, EndTime: end})
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	for _, m := range []string{MetricAvgDuration, MetricLLMLatency, MetricToolSuccessRate, MetricApprovalPassRate, MetricErrorRate} {
		if v := resp.Summary[m]; v != 0 {
			t.Errorf("empty-window metric %s = %v, want 0", m, v)
		}
	}
	// prev=0 且 cur=0:comparison 必须为 0(不能是 Inf,否则 JSON 序列化失败)。
	if _, err := json.Marshal(resp); err != nil {
		t.Fatalf("marshal response: %v", err)
	}
}

func TestGenerateReportValidation(t *testing.T) {
	svc := NewService(&fakeSource{})
	start, end := testWindow()
	cases := []struct {
		name string
		req  ReportRequest
	}{
		{"zero start", ReportRequest{EndTime: end}},
		{"start after end", ReportRequest{StartTime: end, EndTime: start}},
		{"unknown filter", ReportRequest{StartTime: start, EndTime: end, Filters: map[string]string{"sql": "1=1"}}},
		{"unknown metric", ReportRequest{StartTime: start, EndTime: end, Metrics: []string{"nope"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.GenerateReport(context.Background(), "tenant-1", tc.req); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if _, err := svc.GenerateReport(context.Background(), "", ReportRequest{StartTime: start, EndTime: end}); err == nil {
		t.Fatal("expected tenant error")
	}
}

func TestGenerateReportMetricSelection(t *testing.T) {
	svc := NewService(&fakeSource{})
	start, end := testWindow()
	resp, err := svc.GenerateReport(context.Background(), "tenant-1", ReportRequest{
		StartTime: start, EndTime: end, Metrics: []string{MetricTotalTokens},
	})
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	if len(resp.Summary) != 1 || len(resp.Details) != 1 {
		t.Fatalf("expected 1 selected metric, got summary=%d details=%d", len(resp.Summary), len(resp.Details))
	}
}

func TestHandlerGenerateReport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&fakeSource{})
	h := NewHandler(svc)

	start, end := testWindow()
	body := `{"start_time":"` + start.Format(time.RFC3339) + `","end_time":"` + end.Format(time.RFC3339) + `"}`

	t.Run("ok", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/eval/reports", ioReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("tenant_id", "tenant-1")
		h.GenerateReport(c)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
		}
		var resp struct {
			Data ReportResponse `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resp.Data.Summary) != len(AllMetrics) {
			t.Fatalf("expected %d metrics, got %d", len(AllMetrics), len(resp.Data.Summary))
		}
	})

	t.Run("invalid window returns 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/eval/reports", ioReader(`{"start_time":"2026-08-02T00:00:00Z","end_time":"2026-08-01T00:00:00Z"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("tenant_id", "tenant-1")
		h.GenerateReport(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func ioReader(s string) *strings.Reader { return strings.NewReader(s) }
