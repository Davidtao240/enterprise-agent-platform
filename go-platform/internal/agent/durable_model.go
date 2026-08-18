package agent

import (
	"errors"
	"time"
)

const (
	RunStatusQueued          = "queued"
	RunStatusRunning         = "running"
	RunStatusWaitingHuman    = "waiting_human"
	RunStatusWaitingExternal = "waiting_external"
	RunStatusSucceeded       = "succeeded"
	RunStatusFailed          = "failed"
	RunStatusCancelled       = "cancelled"
)

const (
	StepTypeModel      = "model"
	StepTypeTool       = "tool"
	StepTypeCheckpoint = "checkpoint"
	StepTypeInterrupt  = "interrupt"
	StepTypeSystem     = "system"
)

const (
	StepStatusPending   = "pending"
	StepStatusRunning   = "running"
	StepStatusSucceeded = "succeeded"
	StepStatusFailed    = "failed"
	StepStatusCancelled = "cancelled"
)

const (
	RuntimeEventRunQueued       = "run.queued"
	RuntimeEventRunStarted      = "run.started"
	RuntimeEventRunInterrupted  = "run.interrupted"
	RuntimeEventRunResumed      = "run.resumed"
	RuntimeEventRunSucceeded    = "run.succeeded"
	RuntimeEventRunFailed       = "run.failed"
	RuntimeEventRunCancelled    = "run.cancelled"
	RuntimeEventStepStarted     = "step.started"
	RuntimeEventStepCompleted   = "step.completed"
	RuntimeEventStepFailed      = "step.failed"
	RuntimeEventCheckpointSaved = "checkpoint.saved"
)

var (
	ErrDurableRunNotFound        = errors.New("durable run not found")
	ErrInvalidRunTransition      = errors.New("invalid durable run state transition")
	ErrStaleRunAttempt           = errors.New("stale durable run attempt")
	ErrCheckpointVersionConflict = errors.New("checkpoint version conflict")
	ErrRuntimeEventConflict      = errors.New("runtime event identity conflict")
	// ErrLeaseNotHeld 表示迟到结果被拒绝(M1-C-B):Run 的执行 lease 已被
	// 其他执行者持有,非持有者的终态写入不得覆盖接管者的执行。
	ErrLeaseNotHeld = errors.New("durable run lease not held")
)

type AgentThread struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id"`
	CreatedBy          string    `json:"created_by"`
	BusinessAppCode    *string   `json:"business_app_code,omitempty"`
	WorkflowInstanceID *string   `json:"workflow_instance_id,omitempty"`
	Title              *string   `json:"title,omitempty"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type DurableRun struct {
	ID                        string     `json:"id"`
	ThreadID                  string     `json:"thread_id"`
	TenantID                  string     `json:"tenant_id"`
	TraceID                   string     `json:"trace_id"`
	WorkflowInstanceID        *string    `json:"workflow_instance_id,omitempty"`
	NodeInstanceID            *string    `json:"node_instance_id,omitempty"`
	ParentRunID               *string    `json:"parent_run_id,omitempty"`
	GraphKey                  string     `json:"graph_key"`
	GraphVersion              string     `json:"graph_version"`
	ConfigurationSnapshotJSON string     `json:"configuration_snapshot_json"`
	Status                    string     `json:"status"`
	Attempt                   int        `json:"attempt"`
	CheckpointVersion         *int64     `json:"checkpoint_version,omitempty"`
	LeaseOwner                *string    `json:"lease_owner,omitempty"`
	LeaseExpiresAt            *time.Time `json:"lease_expires_at,omitempty"`
	HeartbeatAt               *time.Time `json:"heartbeat_at,omitempty"`
	BudgetJSON                *string    `json:"budget_json,omitempty"`
	OutputSummaryJSON         *string    `json:"output_summary_json,omitempty"`
	UsageJSON                 *string    `json:"usage_json,omitempty"`
	ErrorJSON                 *string    `json:"error_json,omitempty"`
	StartedAt                 *time.Time `json:"started_at,omitempty"`
	FinishedAt                *time.Time `json:"finished_at,omitempty"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

type AgentRunStep struct {
	ID                string     `json:"id"`
	TenantID          string     `json:"tenant_id"`
	RunID             string     `json:"run_id"`
	Sequence          int64      `json:"sequence"`
	Attempt           int        `json:"attempt"`
	StepType          string     `json:"step_type"`
	Name              *string    `json:"name,omitempty"`
	Status            string     `json:"status"`
	InputSummaryJSON  *string    `json:"input_summary_json,omitempty"`
	OutputSummaryJSON *string    `json:"output_summary_json,omitempty"`
	UsageJSON         *string    `json:"usage_json,omitempty"`
	ErrorJSON         *string    `json:"error_json,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type RuntimeEvent struct {
	EventID           string     `json:"event_id"`
	TenantID          string     `json:"tenant_id"`
	RunID             string     `json:"run_id"`
	Sequence          int64      `json:"sequence"`
	Attempt           int        `json:"attempt"`
	EventType         string     `json:"type"`
	PayloadJSON       *string    `json:"payload_json,omitempty"`
	CheckpointVersion *int64     `json:"checkpoint_version,omitempty"`
	OccurredAt        time.Time  `json:"occurred_at"`
	ConsumedAt        *time.Time `json:"consumed_at,omitempty"`
}

// V1DurableRunStart contains trusted control-plane data for the synchronous V1 bridge.
type V1DurableRunStart struct {
	RunID                     string
	TenantID                  string
	CreatedBy                 string
	BusinessAppCode           string
	WorkflowInstanceID        string
	NodeInstanceID            string
	ThreadTitle               string
	TraceID                   string
	GraphKey                  string
	GraphVersion              string
	ConfigurationSnapshotJSON string
	Attempt                   int
	InputSummaryJSON          *string
	StartedAt                 time.Time
}

type V1DurableRunCompletion struct {
	TenantID string
	RunID    string
	Attempt  int
	Status   string
	// LeaseOwner 是本次执行尝试的 lease owner token(M1-C-B)。
	// 非空时,若 Run 非终态且 lease 已被其他 owner 持有,完成写入将被
	// ErrLeaseNotHeld 拒绝(迟到结果拒绝)。空值表示不校验(兼容旧路径)。
	LeaseOwner        string
	OutputSummaryJSON *string
	UsageJSON         *string
	ErrorJSON         *string
	FinishedAt        time.Time
	DurationMs        int
}

// V2DurableRunStart is the control-plane record created before Python accepts
// an asynchronous Runtime V2 Start request.
type V2DurableRunStart struct {
	RunID                     string
	TenantID                  string
	CreatedBy                 string
	BusinessAppCode           string
	WorkflowInstanceID        string
	NodeInstanceID            string
	ThreadTitle               string
	TraceID                   string
	GraphKey                  string
	GraphVersion              string
	ConfigurationSnapshotJSON string
	BudgetJSON                *string
	Attempt                   int
}

// StaleV1Run is the minimal control-plane view used by the convergence
// scanner to re-enqueue node tasks whose durable Run outlived the execution
// threshold (M1-C 前导的失联收敛;后续由 lease/heartbeat 接管)。
type StaleV1Run struct {
	RunID              string
	TenantID           string
	WorkflowInstanceID string
	NodeInstanceID     string
	Attempt            int
	GraphKey           string
}
