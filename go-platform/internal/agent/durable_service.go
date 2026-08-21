package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type durableRunStore interface {
	StartV1RunTx(ctx context.Context, start *V1DurableRunStart) (*DurableRun, bool, error)
	StartV2RunTx(ctx context.Context, start *V2DurableRunStart) (*DurableRun, bool, error)
	CompleteV1RunTx(ctx context.Context, completion *V1DurableRunCompletion) error
	FindDurableRunByIDForTenant(ctx context.Context, tenantID, runID string) (*DurableRun, error)
	TransitionDurableRun(ctx context.Context, tenantID, runID string, attempt int, fromStatuses []string, toStatus string) error
	ApplyRuntimeEventTx(ctx context.Context, event *RuntimeEvent, fromStatuses []string, toStatus string) (bool, error)
	AcquireRunLease(ctx context.Context, tenantID, runID string, attempt int, owner string, ttl time.Duration) (bool, error)
	HeartbeatRunLease(ctx context.Context, tenantID, runID string, attempt int, owner string, ttl time.Duration) (bool, error)
}

type durableRunLifecycle interface {
	StartV1Run(ctx context.Context, start *V1DurableRunStart) (*DurableRun, bool, error)
	CompleteV1Run(ctx context.Context, completion *V1DurableRunCompletion) error
	FindRun(ctx context.Context, tenantID, runID string) (*DurableRun, error)
	AcquireRunLease(ctx context.Context, tenantID, runID string, attempt int, owner string, ttl time.Duration) (bool, error)
	HeartbeatRunLease(ctx context.Context, tenantID, runID string, attempt int, owner string, ttl time.Duration) (bool, error)
}

// DurableRunService owns business-domain-neutral Run transition and idempotency rules.
// The repository keeps multi-record V1 compatibility writes transactional.
type DurableRunService struct {
	store durableRunStore
}

func NewDurableRunService(store durableRunStore) *DurableRunService {
	return &DurableRunService{store: store}
}

func (s *DurableRunService) StartV1Run(ctx context.Context, start *V1DurableRunStart) (*DurableRun, bool, error) {
	if start == nil {
		return nil, false, fmt.Errorf("start durable run: nil request")
	}
	if start.TenantID == "" || start.CreatedBy == "" || start.RunID == "" || start.TraceID == "" ||
		start.BusinessAppCode == "" {
		return nil, false, fmt.Errorf("start durable run: trusted identity fields are required")
	}
	if start.ThreadID == "" && (start.WorkflowInstanceID == "" || start.NodeInstanceID == "") {
		return nil, false, fmt.Errorf("start durable run: workflow identity is required (unless using existing ThreadID)")
	}
	if start.Attempt <= 0 {
		return nil, false, fmt.Errorf("start durable run: attempt must be positive")
	}
	if start.GraphKey == "" || start.GraphVersion == "" {
		return nil, false, fmt.Errorf("start durable run: graph identity is required")
	}
	return s.store.StartV1RunTx(ctx, start)
}

func (s *DurableRunService) StartV2Run(ctx context.Context, start *V2DurableRunStart) (*DurableRun, bool, error) {
	if start == nil {
		return nil, false, fmt.Errorf("start durable V2 run: nil request")
	}
	// M5-C: 实验 Run(shadow/replay 标记)为独立 Run,允许缺少 workflow/node 归属
	// (TRACE_AND_EVAL.md §4.6:不触发工作流节点推进)。
	experiment := isExperimentMetadata(start.MetadataJSON)
	if start.TenantID == "" || start.CreatedBy == "" || start.RunID == "" || start.TraceID == "" ||
		start.BusinessAppCode == "" {
		return nil, false, fmt.Errorf("start durable V2 run: trusted identity fields are required")
	}
	if !experiment && (start.WorkflowInstanceID == "" || start.NodeInstanceID == "") {
		return nil, false, fmt.Errorf("start durable V2 run: workflow identity is required")
	}
	if start.Attempt <= 0 {
		return nil, false, fmt.Errorf("start durable V2 run: attempt must be positive")
	}
	if start.GraphKey == "" || start.GraphVersion == "" {
		return nil, false, fmt.Errorf("start durable V2 run: graph identity is required")
	}
	return s.store.StartV2RunTx(ctx, start)
}

func (s *DurableRunService) CompleteV1Run(ctx context.Context, completion *V1DurableRunCompletion) error {
	if completion == nil {
		return fmt.Errorf("complete durable run: nil request")
	}
	if completion.TenantID == "" || completion.RunID == "" || completion.Attempt <= 0 {
		return fmt.Errorf("complete durable run: trusted identity and attempt are required")
	}
	if completion.Status != RunStatusSucceeded && completion.Status != RunStatusFailed && completion.Status != RunStatusCancelled {
		return fmt.Errorf("%w: V1 completion cannot enter %q", ErrInvalidRunTransition, completion.Status)
	}
	return s.store.CompleteV1RunTx(ctx, completion)
}

func (s *DurableRunService) FindRun(ctx context.Context, tenantID, runID string) (*DurableRun, error) {
	if tenantID == "" || runID == "" {
		return nil, ErrDurableRunNotFound
	}
	return s.store.FindDurableRunByIDForTenant(ctx, tenantID, runID)
}

// AcquireRunLease 以原子 CAS 获取 Run 执行 lease(M1-C-A)。
// 只有"无主、已过期、或持有者正是 owner"时才能获取;每个执行尝试使用
// 唯一 owner token,防止同进程并发重复投递互相穿透。
func (s *DurableRunService) AcquireRunLease(ctx context.Context, tenantID, runID string, attempt int, owner string, ttl time.Duration) (bool, error) {
	if tenantID == "" || runID == "" || owner == "" {
		return false, fmt.Errorf("acquire run lease: trusted identity and owner are required")
	}
	if attempt <= 0 || ttl <= 0 {
		return false, fmt.Errorf("acquire run lease: attempt and ttl must be positive")
	}
	return s.store.AcquireRunLease(ctx, tenantID, runID, attempt, owner, ttl)
}

// HeartbeatRunLease 延长 lease;lease 已被接管(owner 不匹配)时返回 false。
func (s *DurableRunService) HeartbeatRunLease(ctx context.Context, tenantID, runID string, attempt int, owner string, ttl time.Duration) (bool, error) {
	if tenantID == "" || runID == "" || owner == "" {
		return false, fmt.Errorf("heartbeat run lease: trusted identity and owner are required")
	}
	if attempt <= 0 || ttl <= 0 {
		return false, fmt.Errorf("heartbeat run lease: attempt and ttl must be positive")
	}
	return s.store.HeartbeatRunLease(ctx, tenantID, runID, attempt, owner, ttl)
}

func (s *DurableRunService) TransitionRun(ctx context.Context, tenantID, runID string, attempt int, toStatus string) error {
	run, err := s.FindRun(ctx, tenantID, runID)
	if err != nil {
		return err
	}
	if run.Attempt != attempt {
		return fmt.Errorf("%w: current=%d received=%d", ErrStaleRunAttempt, run.Attempt, attempt)
	}
	if !CanTransitionRun(run.Status, toStatus) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidRunTransition, run.Status, toStatus)
	}
	return s.store.TransitionDurableRun(ctx, tenantID, runID, attempt, []string{run.Status}, toStatus)
}

// ApplyRuntimeEvent persists an at-least-once Runtime Event and applies its Run
// transition atomically. Duplicate event IDs or sequences return applied=false.
func (s *DurableRunService) ApplyRuntimeEvent(ctx context.Context, event *RuntimeEvent) (bool, error) {
	if event == nil || event.EventID == "" || event.TenantID == "" || event.RunID == "" {
		return false, fmt.Errorf("apply runtime event: trusted identity fields are required")
	}
	if event.Sequence <= 0 || event.Attempt <= 0 {
		return false, fmt.Errorf("apply runtime event: sequence and attempt must be positive")
	}

	toStatus, fromStatuses := runTransitionForEvent(event)
	return s.store.ApplyRuntimeEventTx(ctx, event, fromStatuses, toStatus)
}

func CanTransitionRun(fromStatus, toStatus string) bool {
	allowed := map[string]map[string]bool{
		RunStatusQueued: {
			RunStatusRunning:   true,
			RunStatusFailed:    true,
			RunStatusCancelled: true,
		},
		RunStatusRunning: {
			RunStatusWaitingHuman:    true,
			RunStatusWaitingExternal: true,
			RunStatusSucceeded:       true,
			RunStatusFailed:          true,
			RunStatusCancelled:       true,
		},
		RunStatusWaitingHuman: {
			RunStatusRunning:   true,
			RunStatusFailed:    true,
			RunStatusCancelled: true,
		},
		RunStatusWaitingExternal: {
			RunStatusRunning:   true,
			RunStatusFailed:    true,
			RunStatusCancelled: true,
		},
	}
	return allowed[fromStatus][toStatus]
}

func runTransitionForEvent(event *RuntimeEvent) (string, []string) {
	switch event.EventType {
	case RuntimeEventRunStarted:
		return RunStatusRunning, []string{RunStatusQueued}
	case RuntimeEventRunInterrupted:
		waitStatus := RunStatusWaitingHuman
		if event.PayloadJSON != nil {
			var payload struct {
				WaitStatus string `json:"wait_status"`
			}
			if json.Unmarshal([]byte(*event.PayloadJSON), &payload) == nil && payload.WaitStatus == RunStatusWaitingExternal {
				waitStatus = RunStatusWaitingExternal
			}
		}
		return waitStatus, []string{RunStatusRunning}
	case RuntimeEventRunResumed:
		return RunStatusRunning, []string{RunStatusWaitingHuman, RunStatusWaitingExternal}
	case RuntimeEventRunSucceeded:
		return RunStatusSucceeded, []string{RunStatusRunning}
	case RuntimeEventRunFailed:
		return RunStatusFailed, []string{RunStatusQueued, RunStatusRunning, RunStatusWaitingHuman, RunStatusWaitingExternal}
	case RuntimeEventRunCancelled:
		return RunStatusCancelled, []string{RunStatusQueued, RunStatusRunning, RunStatusWaitingHuman, RunStatusWaitingExternal}
	default:
		return "", nil
	}
}
