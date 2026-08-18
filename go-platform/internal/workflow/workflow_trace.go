package workflow

import (
	"context"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/trace"
)

// traceSink M5-A: L1 Workflow Trace 事件投递 (由 internal/trace.Recorder 实现)。
type traceSink interface {
	Record(ctx context.Context, events ...*trace.Event)
}

// SetTraceSink 注入 M5-A L1 Workflow 层级 Trace 记录器。
func (s *Service) SetTraceSink(t traceSink) { s.tracer = t }

// traceInstance 记录一条 L1 事件 (start/end/error)。
// trace_id 为工作流实例级 TraceID,与 Run/ToolCall 层共享同一因果链。
func (s *Service) traceInstance(ctx context.Context, inst *Instance, eventType, status string) {
	if s.tracer == nil || inst == nil || inst.TraceID == "" {
		return
	}
	s.tracer.Record(ctx, &trace.Event{
		TraceID:   inst.TraceID,
		Layer:     trace.LayerWorkflow,
		EventType: eventType,
		TenantID:  inst.TenantID,
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"workflow_instance_id": inst.ID,
			"status":               status,
		},
	})
}
