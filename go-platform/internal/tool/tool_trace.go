package tool

import (
	"context"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/trace"
)

// toolTraceSink M5-A: L4 Tool Call Trace 事件投递 (由 internal/trace.Recorder 实现)。
type toolTraceSink interface {
	Record(ctx context.Context, events ...*trace.Event)
}

// WithTraceSink 注入 Trace Recorder(M5-A: L4 Tool Call 层级追踪)。
func WithTraceSink(t toolTraceSink) ServiceOption {
	return func(s *Service) { s.tracer = t }
}

// recordToolTrace 记录一条 L4 事件。trace_id 缺失时退化为 run_id。
func (s *Service) recordToolTrace(ctx context.Context, tc *ToolCall, eventType string) {
	if s.tracer == nil || tc == nil {
		return
	}
	traceID := tc.TraceID
	if traceID == "" {
		traceID = tc.RunID
	}
	if traceID == "" {
		return
	}
	s.tracer.Record(ctx, &trace.Event{
		TraceID:   traceID,
		Layer:     trace.LayerToolCall,
		EventType: eventType,
		TenantID:  tc.TenantID,
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"tool_call_id": tc.ID,
			"tool_id":      tc.ToolID,
			"status":       tc.Status,
		},
	})
}
