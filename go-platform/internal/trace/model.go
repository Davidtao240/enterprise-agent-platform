// Package trace 实现六层全链路追踪 (M5-A)。
//
// 层级定义 (TRACE_AND_EVAL.md §2):
//
//	L1 Workflow      workflow_instance_id
//	L2 Agent Run     agent_run_id
//	L3 Model Turn    agent_run_id + step (Python llm.py 上报)
//	L4 Tool Call     tool_call_id (关联 run trace)
//	L5 Checkpoint    checkpoint.saved 事件
//	L6 Interrupt     run.interrupted / run.resumed
//
// 写入路径: 异步 Recorder (缓冲 channel + 批量 insert, best-effort 不阻塞主链路);
// 查询路径: GET /api/v1/traces/:trace_id (trace:read)。
package trace

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Layer 常量。
const (
	LayerWorkflow  = "L1"
	LayerRun       = "L2"
	LayerModelTurn = "L3"
	LayerToolCall  = "L4"
	LayerCheckpnt  = "L5"
	LayerInterrupt = "L6"
)

// Event 一条 Trace 事件 (内存表示;持久化见 repository)。
type Event struct {
	TraceID     string            `json:"trace_id"`
	Layer       string            `json:"layer"`
	ParentID    *string           `json:"parent_id,omitempty"`
	EventType   string            `json:"event_type"`
	Payload     map[string]any    `json:"payload,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	DurationMs  *int64            `json:"duration_ms,omitempty"`
	TenantID    string            `json:"tenant_id"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
}

// StoredEvent 持久化后的事件 (含数据库生成字段)。
type StoredEvent struct {
	ID          string     `json:"id"`
	TraceID     string     `json:"trace_id"`
	Layer       string     `json:"layer"`
	ParentID    *string    `json:"parent_id,omitempty"`
	EventType   string     `json:"event_type"`
	PayloadJSON *string    `json:"payload_json,omitempty"`
	Timestamp   time.Time  `json:"timestamp"`
	DurationMs  *int64     `json:"duration_ms,omitempty"`
	TenantID    string     `json:"tenant_id"`
	MetadataJSON *string   `json:"metadata_json,omitempty"`
}

// Validate 校验事件必填字段与层级合法性。
func (e *Event) Validate() error {
	if e.TraceID == "" {
		return errors.New("trace_id is required")
	}
	switch e.Layer {
	case LayerWorkflow, LayerRun, LayerModelTurn, LayerToolCall, LayerCheckpnt, LayerInterrupt:
	default:
		return fmt.Errorf("invalid layer %q (expect L1-L6)", e.Layer)
	}
	if e.EventType == "" {
		return errors.New("event_type is required")
	}
	if e.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	return nil
}

// Sink 供其他模块投递 Trace 事件的接口 (异步实现不得阻塞调用方)。
type Sink interface {
	Record(ctx context.Context, events ...*Event)
}
