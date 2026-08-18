package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type fakeDurableRunStore struct {
	runs       map[string]*DurableRun
	attemptIDs map[string]string
	eventIDs   map[string]bool
	eventSeqs  map[string]bool
	leases     map[string]string
}

func newFakeDurableRunStore() *fakeDurableRunStore {
	return &fakeDurableRunStore{
		runs:       map[string]*DurableRun{},
		attemptIDs: map[string]string{},
		eventIDs:   map[string]bool{},
		eventSeqs:  map[string]bool{},
		leases:     map[string]string{},
	}
}

func (f *fakeDurableRunStore) AcquireRunLease(_ context.Context, tenantID, runID string, attempt int, owner string, ttl time.Duration) (bool, error) {
	run := f.runs[runID]
	if run == nil || run.TenantID != tenantID {
		return false, ErrDurableRunNotFound
	}
	if run.Attempt != attempt {
		return false, ErrStaleRunAttempt
	}
	if current := f.leases[runID]; current != "" && current != owner {
		return false, nil
	}
	f.leases[runID] = owner
	return true, nil
}

func (f *fakeDurableRunStore) HeartbeatRunLease(_ context.Context, tenantID, runID string, attempt int, owner string, ttl time.Duration) (bool, error) {
	run := f.runs[runID]
	if run == nil || run.TenantID != tenantID {
		return false, ErrDurableRunNotFound
	}
	if run.Attempt != attempt {
		return false, ErrStaleRunAttempt
	}
	return f.leases[runID] == owner, nil
}

func (f *fakeDurableRunStore) StartV1RunTx(_ context.Context, start *V1DurableRunStart) (*DurableRun, bool, error) {
	attemptKey := fmt.Sprintf("%s:%s:%d", start.TenantID, start.NodeInstanceID, start.Attempt)
	if existingID := f.attemptIDs[attemptKey]; existingID != "" {
		copy := *f.runs[existingID]
		return &copy, false, nil
	}
	run := &DurableRun{
		ID:                        start.RunID,
		TenantID:                  start.TenantID,
		TraceID:                   start.TraceID,
		GraphKey:                  start.GraphKey,
		GraphVersion:              start.GraphVersion,
		ConfigurationSnapshotJSON: start.ConfigurationSnapshotJSON,
		Status:                    RunStatusRunning,
		Attempt:                   start.Attempt,
		StartedAt:                 &start.StartedAt,
	}
	f.runs[run.ID] = run
	f.attemptIDs[attemptKey] = run.ID
	copy := *run
	return &copy, true, nil
}

func (f *fakeDurableRunStore) StartV2RunTx(_ context.Context, start *V2DurableRunStart) (*DurableRun, bool, error) {
	attemptKey := fmt.Sprintf("%s:%s:%d", start.TenantID, start.NodeInstanceID, start.Attempt)
	if existingID := f.attemptIDs[attemptKey]; existingID != "" {
		copy := *f.runs[existingID]
		return &copy, false, nil
	}
	run := &DurableRun{
		ID: start.RunID, TenantID: start.TenantID, TraceID: start.TraceID,
		GraphKey: start.GraphKey, GraphVersion: start.GraphVersion,
		ConfigurationSnapshotJSON: start.ConfigurationSnapshotJSON,
		Status:                    RunStatusQueued, Attempt: start.Attempt,
	}
	f.runs[run.ID] = run
	f.attemptIDs[attemptKey] = run.ID
	copy := *run
	return &copy, true, nil
}

func (f *fakeDurableRunStore) CompleteV1RunTx(_ context.Context, completion *V1DurableRunCompletion) error {
	run := f.runs[completion.RunID]
	if run == nil || run.TenantID != completion.TenantID {
		return ErrDurableRunNotFound
	}
	if run.Attempt != completion.Attempt {
		return ErrStaleRunAttempt
	}
	if run.Status == completion.Status {
		return nil
	}
	// 镜像仓储的迟到结果拒绝(M1-C-B):非终态且 lease 持有者是他人时拒绝。
	if completion.LeaseOwner != "" && !isTerminalRunStatus(run.Status) {
		if current := f.leases[completion.RunID]; current != "" && current != completion.LeaseOwner {
			return ErrLeaseNotHeld
		}
	}
	if !CanTransitionRun(run.Status, completion.Status) {
		return ErrInvalidRunTransition
	}
	run.Status = completion.Status
	run.OutputSummaryJSON = completion.OutputSummaryJSON
	run.ErrorJSON = completion.ErrorJSON
	run.FinishedAt = &completion.FinishedAt
	return nil
}

func (f *fakeDurableRunStore) FindDurableRunByIDForTenant(_ context.Context, tenantID, runID string) (*DurableRun, error) {
	run := f.runs[runID]
	if run == nil || run.TenantID != tenantID {
		return nil, ErrDurableRunNotFound
	}
	copy := *run
	return &copy, nil
}

func (f *fakeDurableRunStore) TransitionDurableRun(_ context.Context, tenantID, runID string, attempt int, fromStatuses []string, toStatus string) error {
	run := f.runs[runID]
	if run == nil || run.TenantID != tenantID {
		return ErrDurableRunNotFound
	}
	if run.Attempt != attempt {
		return ErrStaleRunAttempt
	}
	if !containsStatus(fromStatuses, run.Status) {
		return ErrInvalidRunTransition
	}
	run.Status = toStatus
	return nil
}

func (f *fakeDurableRunStore) ApplyRuntimeEventTx(_ context.Context, event *RuntimeEvent, fromStatuses []string, toStatus string) (bool, error) {
	run := f.runs[event.RunID]
	if run == nil || run.TenantID != event.TenantID {
		return false, ErrDurableRunNotFound
	}
	if run.Attempt != event.Attempt {
		return false, ErrStaleRunAttempt
	}
	sequenceKey := fmt.Sprintf("%s:%d", event.RunID, event.Sequence)
	if f.eventIDs[event.EventID] || f.eventSeqs[sequenceKey] {
		return false, nil
	}
	if toStatus != "" && !containsStatus(fromStatuses, run.Status) {
		return false, ErrInvalidRunTransition
	}
	f.eventIDs[event.EventID] = true
	f.eventSeqs[sequenceKey] = true
	if toStatus != "" {
		run.Status = toStatus
	}
	return true, nil
}

func TestDurableRunStateTransitions(t *testing.T) {
	if !CanTransitionRun(RunStatusQueued, RunStatusRunning) {
		t.Fatal("queued -> running should be allowed")
	}
	if !CanTransitionRun(RunStatusRunning, RunStatusWaitingHuman) {
		t.Fatal("running -> waiting_human should be allowed")
	}
	if CanTransitionRun(RunStatusSucceeded, RunStatusRunning) {
		t.Fatal("terminal succeeded run must not transition back to running")
	}

	store := newFakeDurableRunStore()
	store.runs["run-terminal"] = &DurableRun{
		ID: "run-terminal", TenantID: "tenant-a", Status: RunStatusSucceeded, Attempt: 1,
	}
	service := NewDurableRunService(store)
	err := service.TransitionRun(context.Background(), "tenant-a", "run-terminal", 1, RunStatusRunning)
	if !errors.Is(err, ErrInvalidRunTransition) {
		t.Fatalf("terminal transition error = %v, want ErrInvalidRunTransition", err)
	}
}

func TestDurableRunTenantIsolation(t *testing.T) {
	store := newFakeDurableRunStore()
	store.runs["run-a"] = &DurableRun{
		ID: "run-a", TenantID: "tenant-a", Status: RunStatusRunning, Attempt: 1,
	}
	service := NewDurableRunService(store)

	if _, err := service.FindRun(context.Background(), "tenant-a", "run-a"); err != nil {
		t.Fatalf("same-tenant lookup failed: %v", err)
	}
	if _, err := service.FindRun(context.Background(), "tenant-b", "run-a"); !errors.Is(err, ErrDurableRunNotFound) {
		t.Fatalf("cross-tenant lookup error = %v, want ErrDurableRunNotFound", err)
	}
}

func TestDuplicateRunAttemptReusesExistingRun(t *testing.T) {
	store := newFakeDurableRunStore()
	service := NewDurableRunService(store)
	start := &V1DurableRunStart{
		RunID: "run-1", TenantID: "tenant-a", CreatedBy: "user-a",
		TraceID: "trace-a", BusinessAppCode: "finance",
		WorkflowInstanceID: "workflow-a", NodeInstanceID: "node-a",
		GraphKey: "graph-a", GraphVersion: "1.0.0", Attempt: 1, StartedAt: time.Now(),
	}

	first, created, err := service.StartV1Run(context.Background(), start)
	if err != nil || !created {
		t.Fatalf("first start = (%#v, %v, %v), want created", first, created, err)
	}
	duplicate := *start
	duplicate.RunID = "run-duplicate"
	second, created, err := service.StartV1Run(context.Background(), &duplicate)
	if err != nil || created {
		t.Fatalf("duplicate start = (%#v, %v, %v), want reused", second, created, err)
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate attempt returned run %s, want %s", second.ID, first.ID)
	}

	retry := *start
	retry.RunID = "run-2"
	retry.Attempt = 2
	third, created, err := service.StartV1Run(context.Background(), &retry)
	if err != nil || !created || third.ID == first.ID {
		t.Fatalf("new attempt = (%#v, %v, %v), want a new run", third, created, err)
	}
}

func TestRuntimeEventDuplicateAndStaleAttempt(t *testing.T) {
	store := newFakeDurableRunStore()
	store.runs["run-a"] = &DurableRun{
		ID: "run-a", TenantID: "tenant-a", Status: RunStatusQueued, Attempt: 1,
	}
	service := NewDurableRunService(store)
	event := &RuntimeEvent{
		EventID: "event-1", TenantID: "tenant-a", RunID: "run-a",
		Sequence: 1, Attempt: 1, EventType: RuntimeEventRunStarted, OccurredAt: time.Now(),
	}

	applied, err := service.ApplyRuntimeEvent(context.Background(), event)
	if err != nil || !applied {
		t.Fatalf("first event = (%v, %v), want applied", applied, err)
	}
	applied, err = service.ApplyRuntimeEvent(context.Background(), event)
	if err != nil || applied {
		t.Fatalf("duplicate event = (%v, %v), want idempotent no-op", applied, err)
	}

	late := *event
	late.EventID = "event-late"
	late.Sequence = 2
	late.Attempt = 2
	if _, err := service.ApplyRuntimeEvent(context.Background(), &late); !errors.Is(err, ErrStaleRunAttempt) {
		t.Fatalf("late attempt error = %v, want ErrStaleRunAttempt", err)
	}

	crossTenant := *event
	crossTenant.EventID = "event-cross-tenant"
	crossTenant.Sequence = 3
	crossTenant.TenantID = "tenant-b"
	if _, err := service.ApplyRuntimeEvent(context.Background(), &crossTenant); !errors.Is(err, ErrDurableRunNotFound) {
		t.Fatalf("cross-tenant event error = %v, want ErrDurableRunNotFound", err)
	}
}

func TestRuntimeInterruptTransitionsByWaitKind(t *testing.T) {
	store := newFakeDurableRunStore()
	store.runs["run-human"] = &DurableRun{
		ID: "run-human", TenantID: "tenant-a", Status: RunStatusRunning, Attempt: 1,
	}
	store.runs["run-external"] = &DurableRun{
		ID: "run-external", TenantID: "tenant-a", Status: RunStatusRunning, Attempt: 1,
	}
	service := NewDurableRunService(store)

	humanPayload := `{"interrupt_id":"interrupt-human","kind":"human","wait_status":"waiting_human","resume_schema":{}}`
	applied, err := service.ApplyRuntimeEvent(context.Background(), &RuntimeEvent{
		EventID: "event-human", TenantID: "tenant-a", RunID: "run-human",
		Sequence: 1, Attempt: 1, EventType: RuntimeEventRunInterrupted,
		PayloadJSON: &humanPayload, OccurredAt: time.Now(),
	})
	if err != nil || !applied || store.runs["run-human"].Status != RunStatusWaitingHuman {
		t.Fatalf("human interrupt = (%v, %v, %s)", applied, err, store.runs["run-human"].Status)
	}

	externalPayload := `{"interrupt_id":"interrupt-external","kind":"external","wait_status":"waiting_external","resume_schema":{}}`
	applied, err = service.ApplyRuntimeEvent(context.Background(), &RuntimeEvent{
		EventID: "event-external", TenantID: "tenant-a", RunID: "run-external",
		Sequence: 1, Attempt: 1, EventType: RuntimeEventRunInterrupted,
		PayloadJSON: &externalPayload, OccurredAt: time.Now(),
	})
	if err != nil || !applied || store.runs["run-external"].Status != RunStatusWaitingExternal {
		t.Fatalf("external interrupt = (%v, %v, %s)", applied, err, store.runs["run-external"].Status)
	}
}

func TestDurableRunLeaseAcquireAndHeartbeat(t *testing.T) {
	store := newFakeDurableRunStore()
	store.runs["run-a"] = &DurableRun{
		ID: "run-a", TenantID: "tenant-a", Status: RunStatusRunning, Attempt: 1,
	}
	service := NewDurableRunService(store)

	acquired, err := service.AcquireRunLease(context.Background(), "tenant-a", "run-a", 1, "owner-1", time.Minute)
	if err != nil || !acquired {
		t.Fatalf("first acquire = (%v, %v), want acquired", acquired, err)
	}
	acquired, err = service.AcquireRunLease(context.Background(), "tenant-a", "run-a", 1, "owner-2", time.Minute)
	if err != nil || acquired {
		t.Fatalf("second owner acquire = (%v, %v), want rejected", acquired, err)
	}
	if ok, err := service.HeartbeatRunLease(context.Background(), "tenant-a", "run-a", 1, "owner-1", time.Minute); err != nil || !ok {
		t.Fatalf("owner heartbeat = (%v, %v), want ok", ok, err)
	}
	if ok, err := service.HeartbeatRunLease(context.Background(), "tenant-a", "run-a", 1, "owner-2", time.Minute); err != nil || ok {
		t.Fatalf("foreign heartbeat = (%v, %v), want rejected", ok, err)
	}
	if _, err := service.AcquireRunLease(context.Background(), "tenant-a", "run-a", 2, "owner-1", time.Minute); !errors.Is(err, ErrStaleRunAttempt) {
		t.Fatalf("stale attempt acquire error = %v, want ErrStaleRunAttempt", err)
	}
	if _, err := service.AcquireRunLease(context.Background(), "tenant-a", "run-a", 1, "", time.Minute); err == nil {
		t.Fatal("empty owner must be rejected")
	}
	if _, err := service.AcquireRunLease(context.Background(), "tenant-a", "run-a", 1, "owner-1", 0); err == nil {
		t.Fatal("non-positive ttl must be rejected")
	}
}

// TestDurableRunCompleteRejectsForeignLeaseOwner 验证 M1-C-B 迟到结果拒绝:
// lease 被其他执行者持有时,非持有者的终态写入被 ErrLeaseNotHeld 拒绝;
// 持有者本人、同状态幂等重放与空 owner 兼容路径不受影响。
func TestDurableRunCompleteRejectsForeignLeaseOwner(t *testing.T) {
	store := newFakeDurableRunStore()
	store.runs["run-a"] = &DurableRun{
		ID: "run-a", TenantID: "tenant-a", Status: RunStatusRunning, Attempt: 1,
	}
	service := NewDurableRunService(store)

	// 1) 空 owner(兼容旧路径)在 lease 未赋值时允许完成。
	if err := service.CompleteV1Run(context.Background(), &V1DurableRunCompletion{
		TenantID: "tenant-a", RunID: "run-a", Attempt: 1, Status: RunStatusFailed,
		FinishedAt: time.Now(),
	}); err != nil {
		t.Fatalf("legacy completion without lease = %v, want nil", err)
	}

	// 2) 重建 running Run:owner-a 持有 lease,owner-b 的迟到失败被拒绝。
	store.runs["run-b"] = &DurableRun{
		ID: "run-b", TenantID: "tenant-a", Status: RunStatusRunning, Attempt: 1,
	}
	if acquired, err := service.AcquireRunLease(context.Background(), "tenant-a", "run-b", 1, "owner-a", time.Minute); err != nil || !acquired {
		t.Fatalf("acquire = (%v, %v)", acquired, err)
	}
	lateFailure := &V1DurableRunCompletion{
		TenantID: "tenant-a", RunID: "run-b", Attempt: 1,
		Status: RunStatusFailed, LeaseOwner: "owner-b", FinishedAt: time.Now(),
	}
	if err := service.CompleteV1Run(context.Background(), lateFailure); !errors.Is(err, ErrLeaseNotHeld) {
		t.Fatalf("foreign-owner completion = %v, want ErrLeaseNotHeld", err)
	}
	if store.runs["run-b"].Status != RunStatusRunning {
		t.Fatalf("run-b status = %s, want still running (late result must not overwrite)", store.runs["run-b"].Status)
	}

	// 3) 持有者 owner-a 的完成正常生效。
	if err := service.CompleteV1Run(context.Background(), &V1DurableRunCompletion{
		TenantID: "tenant-a", RunID: "run-b", Attempt: 1,
		Status: RunStatusSucceeded, LeaseOwner: "owner-a", FinishedAt: time.Now(),
	}); err != nil {
		t.Fatalf("lease-owner completion = %v, want nil", err)
	}

	// 4) 终态后同状态幂等重放,即使携带错误 owner 也不再改变状态。
	if err := service.CompleteV1Run(context.Background(), &V1DurableRunCompletion{
		TenantID: "tenant-a", RunID: "run-b", Attempt: 1,
		Status: RunStatusSucceeded, LeaseOwner: "owner-b", FinishedAt: time.Now(),
	}); err != nil {
		t.Fatalf("duplicate terminal completion = %v, want nil (idempotent)", err)
	}
	if store.runs["run-b"].Status != RunStatusSucceeded {
		t.Fatalf("run-b status = %s, want succeeded", store.runs["run-b"].Status)
	}
}

// TestTerminalEventPayloadFields 验证 M2-A 终态事件载荷提取:成功事件取
// {output,usage},失败事件取 {error},空载荷/键缺失返回 nil 保留原值。
func TestTerminalEventPayloadFields(t *testing.T) {
	payload := func(value string) *string { return &value }

	// run.succeeded:提取 output/usage,不提取 error。
	succeeded := &RuntimeEvent{
		EventType:   RuntimeEventRunSucceeded,
		PayloadJSON: payload(`{"output":{"summary":"s"},"usage":{"total_tokens":7},"error":{"code":"IGNORED"}}`),
	}
	output, usage, errJSON := terminalEventPayloadFields(succeeded)
	if output == nil || *output != `{"summary":"s"}` {
		t.Fatalf("succeeded output = %v", output)
	}
	if usage == nil || *usage != `{"total_tokens":7}` {
		t.Fatalf("succeeded usage = %v", usage)
	}
	if errJSON != nil {
		t.Fatalf("succeeded error = %v, want nil", errJSON)
	}

	// run.failed:提取 error,不提取 output/usage。
	failed := &RuntimeEvent{
		EventType:   RuntimeEventRunFailed,
		PayloadJSON: payload(`{"error":{"code":"BOOM"},"output":{"summary":"IGNORED"}}`),
	}
	output, usage, errJSON = terminalEventPayloadFields(failed)
	if output != nil || usage != nil {
		t.Fatalf("failed output/usage = (%v, %v), want nil", output, usage)
	}
	if errJSON == nil || *errJSON != `{"code":"BOOM"}` {
		t.Fatalf("failed error = %v", errJSON)
	}

	// 空载荷、null 键、非终态事件、非法 JSON 一律返回 nil(V1 桥兼容)。
	empty := &RuntimeEvent{EventType: RuntimeEventRunSucceeded}
	if output, usage, errJSON = terminalEventPayloadFields(empty); output != nil || usage != nil || errJSON != nil {
		t.Fatalf("empty payload = (%v, %v, %v), want nil", output, usage, errJSON)
	}
	nullKeys := &RuntimeEvent{
		EventType:   RuntimeEventRunSucceeded,
		PayloadJSON: payload(`{"output":null,"usage":null}`),
	}
	if output, usage, errJSON = terminalEventPayloadFields(nullKeys); output != nil || usage != nil || errJSON != nil {
		t.Fatalf("null keys = (%v, %v, %v), want nil", output, usage, errJSON)
	}
	started := &RuntimeEvent{
		EventType:   RuntimeEventRunStarted,
		PayloadJSON: payload(`{"output":{"summary":"IGNORED"}}`),
	}
	if output, usage, errJSON = terminalEventPayloadFields(started); output != nil || usage != nil || errJSON != nil {
		t.Fatalf("non-terminal event = (%v, %v, %v), want nil", output, usage, errJSON)
	}
	broken := &RuntimeEvent{EventType: RuntimeEventRunSucceeded, PayloadJSON: payload(`{not-json`)}
	if output, usage, errJSON = terminalEventPayloadFields(broken); output != nil || usage != nil || errJSON != nil {
		t.Fatalf("broken payload = (%v, %v, %v), want nil", output, usage, errJSON)
	}
}
