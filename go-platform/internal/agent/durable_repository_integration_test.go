package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Run with:
//
//	TEST_DATABASE_URL=postgres://platform:platform_dev@localhost:5432/enterprise_agent_platform \
//	  go test ./internal/agent -run TestDurableRepositoryPostgresAcceptance -v
func TestDurableRepositoryPostgresAcceptance(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect acceptance database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping acceptance database: %v", err)
	}

	var tenantID, userID string
	if err := pool.QueryRow(ctx,
		`SELECT tenant_id, id FROM users ORDER BY created_at LIMIT 1`,
	).Scan(&tenantID, &userID); err != nil {
		t.Fatalf("load acceptance actor: %v", err)
	}
	var templateID, templateKey, templateVersion, graphKey string
	if err := pool.QueryRow(ctx,
		`SELECT id, workflow_template_key, version, graph_key
		 FROM workflow_templates WHERE status = 'active' ORDER BY created_at LIMIT 1`,
	).Scan(&templateID, &templateKey, &templateVersion, &graphKey); err != nil {
		t.Fatalf("load acceptance template: %v", err)
	}

	workflowID := uuid.NewString()
	nodeID := uuid.NewString()
	traceID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_instances
			 (id, tenant_id, business_app_code, workflow_template_id, workflow_template_key,
			  workflow_template_version, graph_key, title, status, input_json, created_by, trace_id)
		 VALUES ($1,$2,'finance',$3,$4,$5,$6,'M1 acceptance','running','{}',$7,$8)`,
		workflowID, tenantID, templateID, templateKey, templateVersion, graphKey, userID, traceID,
	); err != nil {
		t.Fatalf("create acceptance workflow: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_node_instances
			 (id, workflow_instance_id, node_key, node_type, name, status)
		 VALUES ($1,$2,'agent_acceptance','agent_graph','M1 acceptance agent','running')`,
		nodeID, workflowID,
	); err != nil {
		t.Fatalf("create acceptance node: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM runtime_events WHERE run_id IN (SELECT id FROM agent_runs WHERE workflow_instance_id = $1)`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_interrupts WHERE run_id IN (SELECT id FROM agent_runs WHERE workflow_instance_id = $1)`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_checkpoints WHERE run_id IN (SELECT id FROM agent_runs WHERE workflow_instance_id = $1)`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_run_steps WHERE run_id IN (SELECT id FROM agent_runs WHERE workflow_instance_id = $1)`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_run_logs WHERE workflow_instance_id = $1`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE workflow_instance_id = $1`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_threads WHERE workflow_instance_id = $1`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM workflow_node_instances WHERE workflow_instance_id = $1`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM workflow_instances WHERE id = $1`, workflowID)
	}()

	repository := NewRepository(pool)
	service := NewDurableRunService(repository)
	v2RunID := uuid.NewString()
	v2Run, created, err := service.StartV2Run(ctx, &V2DurableRunStart{
		RunID: v2RunID, TenantID: tenantID, CreatedBy: userID,
		BusinessAppCode: "finance", WorkflowInstanceID: workflowID, NodeInstanceID: nodeID,
		ThreadTitle: "M1 acceptance", TraceID: traceID, GraphKey: graphKey,
		GraphVersion: templateVersion, ConfigurationSnapshotJSON: `{"protocol_version":"2.0"}`,
		Attempt: 1,
	})
	if err != nil || !created || v2Run.Status != RunStatusQueued {
		t.Fatalf("StartV2Run = (%#v, %v, %v), want new queued run", v2Run, created, err)
	}
	duplicate, created, err := service.StartV2Run(ctx, &V2DurableRunStart{
		RunID: uuid.NewString(), TenantID: tenantID, CreatedBy: userID,
		BusinessAppCode: "finance", WorkflowInstanceID: workflowID, NodeInstanceID: nodeID,
		ThreadTitle: "M1 acceptance", TraceID: traceID, GraphKey: graphKey,
		GraphVersion: templateVersion, ConfigurationSnapshotJSON: `{"protocol_version":"2.0"}`,
		Attempt: 1,
	})
	if err != nil || created || duplicate.ID != v2Run.ID {
		t.Fatalf("duplicate StartV2Run = (%#v, %v, %v), want same run", duplicate, created, err)
	}

	now := time.Now().UTC()
	apply := func(event *RuntimeEvent) {
		t.Helper()
		applied, applyErr := service.ApplyRuntimeEvent(ctx, event)
		if applyErr != nil || !applied {
			t.Fatalf("ApplyRuntimeEvent(%s) = (%v, %v)", event.EventType, applied, applyErr)
		}
	}
	apply(&RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: v2RunID, Sequence: 1,
		Attempt: 1, EventType: RuntimeEventRunStarted, OccurredAt: now,
	})
	checkpoint1 := int64(1)
	checkpointPayload1 := `{"backend":"sqlite","checkpoint_ref":"langgraph-sqlite:acceptance:1","state_hash":"hash-1"}`
	apply(&RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: v2RunID, Sequence: 2,
		Attempt: 1, EventType: RuntimeEventCheckpointSaved, PayloadJSON: &checkpointPayload1,
		CheckpointVersion: &checkpoint1, OccurredAt: now.Add(time.Millisecond),
	})
	interruptPayload := `{"interrupt_id":"interrupt-acceptance","kind":"human","wait_status":"waiting_human","resume_schema":{"type":"object","required":["decision"]}}`
	interruptEvent := &RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: v2RunID, Sequence: 3,
		Attempt: 1, EventType: RuntimeEventRunInterrupted, PayloadJSON: &interruptPayload,
		CheckpointVersion: &checkpoint1, OccurredAt: now.Add(2 * time.Millisecond),
	}
	apply(interruptEvent)
	sequenceDuplicate := *interruptEvent
	sequenceDuplicate.EventID = uuid.NewString()
	applied, err := service.ApplyRuntimeEvent(ctx, &sequenceDuplicate)
	if err != nil || applied {
		t.Fatalf("sequence duplicate = (%v, %v), want idempotent no-op", applied, err)
	}
	conflictingDuplicate := sequenceDuplicate
	conflictingDuplicate.EventID = uuid.NewString()
	conflictingPayload := `{"interrupt_id":"different-interrupt","kind":"human","wait_status":"waiting_human","resume_schema":{}}`
	conflictingDuplicate.PayloadJSON = &conflictingPayload
	if _, err := service.ApplyRuntimeEvent(ctx, &conflictingDuplicate); !errors.Is(err, ErrRuntimeEventConflict) {
		t.Fatalf("conflicting sequence error = %v, want ErrRuntimeEventConflict", err)
	}
	resumePayload := `{"interrupt_id":"interrupt-acceptance","idempotency_key":"resume-acceptance"}`
	apply(&RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: v2RunID, Sequence: 4,
		Attempt: 1, EventType: RuntimeEventRunResumed, PayloadJSON: &resumePayload,
		CheckpointVersion: &checkpoint1, OccurredAt: now.Add(3 * time.Millisecond),
	})
	checkpoint2 := int64(2)
	checkpointPayload2 := `{"backend":"sqlite","checkpoint_ref":"langgraph-sqlite:acceptance:2","state_hash":"hash-2"}`
	apply(&RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: v2RunID, Sequence: 5,
		Attempt: 1, EventType: RuntimeEventCheckpointSaved, PayloadJSON: &checkpointPayload2,
		CheckpointVersion: &checkpoint2, OccurredAt: now.Add(4 * time.Millisecond),
	})
	apply(&RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: v2RunID, Sequence: 6,
		Attempt: 1, EventType: RuntimeEventRunSucceeded, CheckpointVersion: &checkpoint2,
		OccurredAt: now.Add(5 * time.Millisecond),
	})

	finalRun, err := service.FindRun(ctx, tenantID, v2RunID)
	if err != nil || finalRun.Status != RunStatusSucceeded || finalRun.CheckpointVersion == nil || *finalRun.CheckpointVersion != 2 {
		t.Fatalf("final V2 run = (%#v, %v)", finalRun, err)
	}
	var checkpointCount, eventCount int
	var interruptStatus string
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_checkpoints WHERE run_id = $1`, v2RunID).Scan(&checkpointCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM runtime_events WHERE run_id = $1`, v2RunID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM agent_interrupts WHERE run_id = $1`, v2RunID).Scan(&interruptStatus); err != nil {
		t.Fatal(err)
	}
	if checkpointCount != 2 || eventCount != 6 || interruptStatus != "resumed" {
		t.Fatalf("metadata checkpoints=%d events=%d interrupt=%s", checkpointCount, eventCount, interruptStatus)
	}

	late := &RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: v2RunID, Sequence: 7,
		Attempt: 2, EventType: RuntimeEventRunFailed, OccurredAt: now,
	}
	if _, err := service.ApplyRuntimeEvent(ctx, late); !errors.Is(err, ErrStaleRunAttempt) {
		t.Fatalf("late attempt error = %v, want ErrStaleRunAttempt", err)
	}
	crossTenant := *late
	crossTenant.EventID = uuid.NewString()
	crossTenant.Attempt = 1
	crossTenant.TenantID = uuid.NewString()
	if _, err := service.ApplyRuntimeEvent(ctx, &crossTenant); !errors.Is(err, ErrDurableRunNotFound) {
		t.Fatalf("cross-tenant error = %v, want ErrDurableRunNotFound", err)
	}

	cancelNodeID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_node_instances
			 (id, workflow_instance_id, node_key, node_type, name, status)
		 VALUES ($1,$2,'agent_cancel','agent_graph','M1 cancel acceptance','running')`,
		cancelNodeID, workflowID,
	); err != nil {
		t.Fatalf("create cancel acceptance node: %v", err)
	}
	cancelRunID := uuid.NewString()
	if _, created, err := service.StartV2Run(ctx, &V2DurableRunStart{
		RunID: cancelRunID, TenantID: tenantID, CreatedBy: userID,
		BusinessAppCode: "finance", WorkflowInstanceID: workflowID, NodeInstanceID: cancelNodeID,
		ThreadTitle: "M1 cancel acceptance", TraceID: uuid.NewString(), GraphKey: graphKey,
		GraphVersion: templateVersion, ConfigurationSnapshotJSON: `{"protocol_version":"2.0"}`,
		Attempt: 1,
	}); err != nil || !created {
		t.Fatalf("create cancel acceptance run = (%v, %v)", created, err)
	}
	apply(&RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: cancelRunID, Sequence: 1,
		Attempt: 1, EventType: RuntimeEventRunStarted, OccurredAt: now,
	})
	stepID := uuid.NewString()
	stepStartedPayload := fmt.Sprintf(
		`{"step_id":%q,"step_sequence":1,"step_type":"system","name":"graph_execution"}`,
		stepID,
	)
	apply(&RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: cancelRunID, Sequence: 2,
		Attempt: 1, EventType: RuntimeEventStepStarted, PayloadJSON: &stepStartedPayload,
		OccurredAt: now.Add(time.Millisecond),
	})
	stepFailedPayload := fmt.Sprintf(
		`{"step_id":%q,"step_sequence":1,"step_type":"system","name":"graph_execution","status":"cancelled"}`,
		stepID,
	)
	apply(&RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: cancelRunID, Sequence: 3,
		Attempt: 1, EventType: RuntimeEventStepFailed, PayloadJSON: &stepFailedPayload,
		OccurredAt: now.Add(2 * time.Millisecond),
	})
	apply(&RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: cancelRunID, Sequence: 4,
		Attempt: 1, EventType: RuntimeEventRunCancelled, OccurredAt: now.Add(3 * time.Millisecond),
	})
	var cancelledRunStatus, cancelledStepStatus string
	if err := pool.QueryRow(ctx,
		`SELECT ar.status, ars.status FROM agent_runs ar
		 JOIN agent_run_steps ars ON ars.run_id = ar.id
		 WHERE ar.id = $1 AND ar.tenant_id = $2`, cancelRunID, tenantID,
	).Scan(&cancelledRunStatus, &cancelledStepStatus); err != nil ||
		cancelledRunStatus != RunStatusCancelled || cancelledStepStatus != StepStatusCancelled {
		t.Fatalf("cancelled run/step = (%s, %s, %v)", cancelledRunStatus, cancelledStepStatus, err)
	}

	// V1 compatibility remains transactionally linked to the same durable model.
	v1RunID := uuid.NewString()
	startedAt := time.Now().UTC()
	v1Run, created, err := service.StartV1Run(ctx, &V1DurableRunStart{
		RunID: v1RunID, TenantID: tenantID, CreatedBy: userID,
		BusinessAppCode: "finance", WorkflowInstanceID: workflowID, NodeInstanceID: nodeID,
		ThreadTitle: "M1 acceptance", TraceID: uuid.NewString(), GraphKey: graphKey,
		GraphVersion: templateVersion, ConfigurationSnapshotJSON: `{"protocol_version":"1.0"}`,
		Attempt: 2, StartedAt: startedAt,
	})
	if err != nil || !created || v1Run.Status != RunStatusRunning {
		t.Fatalf("StartV1Run = (%#v, %v, %v)", v1Run, created, err)
	}
	output := `{"summary":"accepted"}`
	if err := service.CompleteV1Run(ctx, &V1DurableRunCompletion{
		TenantID: tenantID, RunID: v1RunID, Attempt: 2, Status: RunStatusSucceeded,
		OutputSummaryJSON: &output, FinishedAt: time.Now().UTC(), DurationMs: 1,
	}); err != nil {
		t.Fatalf("CompleteV1Run: %v", err)
	}
	var durableRunID string
	if err := pool.QueryRow(ctx,
		`SELECT durable_run_id FROM agent_run_logs WHERE run_id = $1 AND tenant_id = $2`,
		v1RunID, tenantID,
	).Scan(&durableRunID); err != nil || durableRunID != v1RunID {
		t.Fatalf("V1 compatibility durable link = (%s, %v)", durableRunID, err)
	}

	agentServiceURL := os.Getenv("TEST_AGENT_SERVICE_URL")
	serviceToken := os.Getenv("TEST_INTERNAL_SERVICE_TOKEN")
	if agentServiceURL == "" || serviceToken == "" {
		t.Log("Runtime V2 cross-service acceptance skipped; TEST_AGENT_SERVICE_URL or TEST_INTERNAL_SERVICE_TOKEN is not set")
		return
	}
	gateway := NewGateway(repository, nil, agentServiceURL, false)
	gateway.ConfigureRuntimeV2(serviceToken)
	v1E2ENodeID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_node_instances
			 (id, workflow_instance_id, node_key, node_type, name, status)
		 VALUES ($1,$2,'agent_v1_e2e','agent_graph','M1-A V1 E2E agent','running')`,
		v1E2ENodeID, workflowID,
	); err != nil {
		t.Fatalf("create V1 E2E node: %v", err)
	}
	v1E2ETraceID := uuid.NewString()
	v1Response, err := gateway.Execute(ctx, &AgentRunPayload{
		TraceID: v1E2ETraceID, BusinessAppCode: "finance",
		WorkflowTemplateKey: templateKey, WorkflowTemplateVersion: templateVersion,
		GraphKey: graphKey, WorkflowInstanceID: workflowID, NodeInstanceID: v1E2ENodeID,
		ThreadTitle: "M1-A V1 cross-service acceptance", Attempt: 1,
		Input: map[string]any{}, UserID: userID, TenantID: tenantID,
	})
	if err != nil || v1Response.Status != RunStatusSucceeded || v1Response.RunID == "" {
		t.Fatalf("Gateway.Execute V1 = (%#v, %v), want succeeded", v1Response, err)
	}
	var v1E2ERunStatus, v1E2ELogStatus string
	if err := pool.QueryRow(ctx,
		`SELECT ar.status, arl.status
		 FROM agent_runs ar JOIN agent_run_logs arl ON arl.durable_run_id = ar.id
		 WHERE ar.tenant_id = $1 AND ar.id = $2 AND ar.node_instance_id = $3`,
		tenantID, v1Response.RunID, v1E2ENodeID,
	).Scan(&v1E2ERunStatus, &v1E2ELogStatus); err != nil ||
		v1E2ERunStatus != RunStatusSucceeded || v1E2ELogStatus != RunStatusSucceeded {
		t.Fatalf("V1 cross-service persistence = (%s, %s, %v)", v1E2ERunStatus, v1E2ELogStatus, err)
	}

	e2eNodeID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_node_instances
			 (id, workflow_instance_id, node_key, node_type, name, status)
		 VALUES ($1,$2,'agent_e2e','agent_graph','M1-B E2E agent','running')`,
		e2eNodeID, workflowID,
	); err != nil {
		t.Fatalf("create Runtime V2 E2E node: %v", err)
	}
	accepted, err := gateway.StartV2(ctx, &AgentRunPayload{
		TraceID: uuid.NewString(), BusinessAppCode: "finance",
		WorkflowTemplateKey: templateKey, WorkflowTemplateVersion: templateVersion,
		GraphKey: graphKey, WorkflowInstanceID: workflowID, NodeInstanceID: e2eNodeID,
		ThreadTitle: "M1-B cross-service acceptance", Attempt: 1,
		Input: map[string]any{}, UserID: userID, TenantID: tenantID,
	}, RuntimeV2Configuration{
		AgentDefinitionVersion: graphKey + "@" + templateVersion,
		ProfileOrSkillVersion:  "finance_operating_report_profile@1.0.0",
		ModelConfigVersion:     "acceptance-model@1.0.0",
	}, RuntimeV2Budget{MaxSteps: 30, MaxCost: 1.0})
	if err != nil || accepted.Status != RunStatusQueued {
		t.Fatalf("Gateway.StartV2 = (%#v, %v), want queued acceptance", accepted, err)
	}
	deadline := time.Now().Add(15 * time.Second)
	var e2eRun *DurableRun
	for time.Now().Before(deadline) {
		e2eRun, err = service.FindRun(ctx, tenantID, accepted.RunID)
		if err == nil && (e2eRun.Status == RunStatusSucceeded || e2eRun.Status == RunStatusFailed) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil || e2eRun == nil || e2eRun.Status != RunStatusSucceeded ||
		e2eRun.CheckpointVersion == nil || *e2eRun.CheckpointVersion != 1 {
		t.Fatalf("Runtime V2 cross-service final run = (%#v, %v)", e2eRun, err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM runtime_events WHERE run_id = $1`, accepted.RunID,
	).Scan(&eventCount); err != nil || eventCount != 5 {
		t.Fatalf("Runtime V2 cross-service events = (%d, %v), want 5", eventCount, err)
	}
	var stepCount int
	var stepStatus string
	if err := pool.QueryRow(ctx,
		`SELECT count(*), min(status) FROM agent_run_steps WHERE run_id = $1`, accepted.RunID,
	).Scan(&stepCount, &stepStatus); err != nil || stepCount != 1 || stepStatus != StepStatusSucceeded {
		t.Fatalf("Runtime V2 cross-service steps = (%d, %s, %v), want one succeeded", stepCount, stepStatus, err)
	}
}

// TestDurableRunLeasePostgresAcceptance verifies M1-C-A lease acquisition,
// heartbeat extension, and expired-lease takeover against real PostgreSQL.
//
//	TEST_DATABASE_URL=... go test ./internal/agent -run TestDurableRunLeasePostgresAcceptance -v
func TestDurableRunLeasePostgresAcceptance(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect acceptance database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping acceptance database: %v", err)
	}

	var tenantID, userID string
	if err := pool.QueryRow(ctx,
		`SELECT tenant_id, id FROM users ORDER BY created_at LIMIT 1`,
	).Scan(&tenantID, &userID); err != nil {
		t.Fatalf("load acceptance actor: %v", err)
	}
	var templateID, templateKey, templateVersion, graphKey string
	if err := pool.QueryRow(ctx,
		`SELECT id, workflow_template_key, version, graph_key
		 FROM workflow_templates WHERE status = 'active' ORDER BY created_at LIMIT 1`,
	).Scan(&templateID, &templateKey, &templateVersion, &graphKey); err != nil {
		t.Fatalf("load acceptance template: %v", err)
	}

	workflowID := uuid.NewString()
	nodeID := uuid.NewString()
	traceID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_instances
			 (id, tenant_id, business_app_code, workflow_template_id, workflow_template_key,
			  workflow_template_version, graph_key, title, status, input_json, created_by, trace_id)
		 VALUES ($1,$2,'finance',$3,$4,$5,$6,'M1-C lease acceptance','running','{}',$7,$8)`,
		workflowID, tenantID, templateID, templateKey, templateVersion, graphKey, userID, traceID,
	); err != nil {
		t.Fatalf("create acceptance workflow: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_node_instances
			 (id, workflow_instance_id, node_key, node_type, name, status)
		 VALUES ($1,$2,'agent_lease','agent_graph','M1-C lease agent','running')`,
		nodeID, workflowID,
	); err != nil {
		t.Fatalf("create acceptance node: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_run_logs WHERE workflow_instance_id = $1`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE workflow_instance_id = $1`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_threads WHERE workflow_instance_id = $1`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM workflow_node_instances WHERE workflow_instance_id = $1`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM workflow_instances WHERE id = $1`, workflowID)
	}()

	repository := NewRepository(pool)
	service := NewDurableRunService(repository)
	runID := uuid.NewString()
	run, created, err := service.StartV2Run(ctx, &V2DurableRunStart{
		RunID: runID, TenantID: tenantID, CreatedBy: userID,
		BusinessAppCode: "finance", WorkflowInstanceID: workflowID, NodeInstanceID: nodeID,
		ThreadTitle: "M1-C lease acceptance", TraceID: traceID, GraphKey: graphKey,
		GraphVersion: templateVersion, ConfigurationSnapshotJSON: `{"protocol_version":"2.0"}`,
		Attempt: 1,
	})
	if err != nil || !created || run.Status != RunStatusQueued {
		t.Fatalf("StartV2Run = (%#v, %v, %v), want new queued run", run, created, err)
	}
	if _, err := service.ApplyRuntimeEvent(ctx, &RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: runID, Sequence: 1,
		Attempt: 1, EventType: RuntimeEventRunStarted, OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("start run for lease acceptance: %v", err)
	}

	// 1) 首个 owner 获取 lease 成功。
	acquired, err := service.AcquireRunLease(ctx, tenantID, runID, 1, "owner-a", time.Hour)
	if err != nil || !acquired {
		t.Fatalf("acquire by owner-a = (%v, %v), want true", acquired, err)
	}
	// 2) 第二个 owner 在 lease 有效期内无法获取。
	acquired, err = service.AcquireRunLease(ctx, tenantID, runID, 1, "owner-b", time.Hour)
	if err != nil || acquired {
		t.Fatalf("acquire by owner-b = (%v, %v), want false", acquired, err)
	}
	// 3) owner-a 心跳续租成功,owner-b 心跳被拒绝。
	ok, err := service.HeartbeatRunLease(ctx, tenantID, runID, 1, "owner-a", time.Hour)
	if err != nil || !ok {
		t.Fatalf("heartbeat owner-a = (%v, %v), want true", ok, err)
	}
	ok, err = service.HeartbeatRunLease(ctx, tenantID, runID, 1, "owner-b", time.Hour)
	if err != nil || ok {
		t.Fatalf("heartbeat owner-b = (%v, %v), want false", ok, err)
	}
	// 4) lease 过期后可被新 owner 接管(直接使 owner-a 的 lease 过期)。
	acquired, err = service.AcquireRunLease(ctx, tenantID, runID, 1, "owner-c", time.Hour)
	if err != nil || acquired {
		t.Fatalf("acquire by owner-c within owner-a lease = (%v, %v), want false", acquired, err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE agent_runs SET lease_expires_at = now() - interval '1 second' WHERE id = $1`, runID,
	); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	acquired, err = service.AcquireRunLease(ctx, tenantID, runID, 1, "owner-c", time.Hour)
	if err != nil || !acquired {
		t.Fatalf("acquire by owner-c after expiry = (%v, %v), want true", acquired, err)
	}
	// 5) 终态 Run 不再允许获取 lease。
	if _, err := service.ApplyRuntimeEvent(ctx, &RuntimeEvent{
		EventID: uuid.NewString(), TenantID: tenantID, RunID: runID, Sequence: 2,
		Attempt: 1, EventType: RuntimeEventRunSucceeded, OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("succeed run for lease acceptance: %v", err)
	}
	acquired, err = service.AcquireRunLease(ctx, tenantID, runID, 1, "owner-d", time.Hour)
	if err != nil || acquired {
		t.Fatalf("acquire on terminal run = (%v, %v), want false", acquired, err)
	}
	// 6) 过期 lease 的失联 Run 出现在接管候选里。
	_, _ = pool.Exec(ctx, `UPDATE agent_runs SET lease_expires_at = now() - interval '1 second' WHERE id = $1`, runID)
	staleRuns, err := repository.ListStaleRunningRuns(ctx, time.Now())
	if err != nil {
		t.Fatalf("ListStaleRunningRuns: %v", err)
	}
	found := false
	for _, candidate := range staleRuns {
		if candidate.RunID == runID {
			found = true
			break
		}
	}
	// 终态 Run 即使 lease 过期也不应成为接管候选。
	if found {
		t.Fatalf("terminal run appeared in takeover candidates: %#v", staleRuns)
	}
}

// TestDurableRunLateResultRejectionPostgresAcceptance verifies M1-C-B late
// result rejection against real PostgreSQL: when a Run's lease is held by a
// takeover executor, a stale executor's terminal completion is rejected with
// ErrLeaseNotHeld and must not overwrite the run state.
//
//	TEST_DATABASE_URL=... go test ./internal/agent -run TestDurableRunLateResultRejectionPostgresAcceptance -v
func TestDurableRunLateResultRejectionPostgresAcceptance(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect acceptance database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping acceptance database: %v", err)
	}

	var tenantID, userID string
	if err := pool.QueryRow(ctx,
		`SELECT tenant_id, id FROM users ORDER BY created_at LIMIT 1`,
	).Scan(&tenantID, &userID); err != nil {
		t.Fatalf("load acceptance actor: %v", err)
	}
	var templateID, templateKey, templateVersion, graphKey string
	if err := pool.QueryRow(ctx,
		`SELECT id, workflow_template_key, version, graph_key
		 FROM workflow_templates WHERE status = 'active' ORDER BY created_at LIMIT 1`,
	).Scan(&templateID, &templateKey, &templateVersion, &graphKey); err != nil {
		t.Fatalf("load acceptance template: %v", err)
	}

	workflowID := uuid.NewString()
	nodeID := uuid.NewString()
	traceID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_instances
			 (id, tenant_id, business_app_code, workflow_template_id, workflow_template_key,
			  workflow_template_version, graph_key, title, status, input_json, created_by, trace_id)
		 VALUES ($1,$2,'finance',$3,$4,$5,$6,'M1-C late result acceptance','running','{}',$7,$8)`,
		workflowID, tenantID, templateID, templateKey, templateVersion, graphKey, userID, traceID,
	); err != nil {
		t.Fatalf("create acceptance workflow: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_node_instances
			 (id, workflow_instance_id, node_key, node_type, name, status)
		 VALUES ($1,$2,'agent_late_result','agent_graph','M1-C late result agent','running')`,
		nodeID, workflowID,
	); err != nil {
		t.Fatalf("create acceptance node: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_run_logs WHERE workflow_instance_id = $1`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM runtime_events WHERE run_id IN (SELECT id FROM agent_runs WHERE workflow_instance_id = $1)`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_run_steps WHERE run_id IN (SELECT id FROM agent_runs WHERE workflow_instance_id = $1)`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE workflow_instance_id = $1`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_threads WHERE workflow_instance_id = $1`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM workflow_node_instances WHERE workflow_instance_id = $1`, workflowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM workflow_instances WHERE id = $1`, workflowID)
	}()

	repository := NewRepository(pool)
	service := NewDurableRunService(repository)
	runID := uuid.NewString()
	_, created, err := service.StartV1Run(ctx, &V1DurableRunStart{
		RunID: runID, TenantID: tenantID, CreatedBy: userID,
		BusinessAppCode: "finance", WorkflowInstanceID: workflowID, NodeInstanceID: nodeID,
		ThreadTitle: "M1-C late result acceptance", TraceID: traceID, GraphKey: graphKey,
		GraphVersion: templateVersion, ConfigurationSnapshotJSON: `{"protocol_version":"1.0"}`,
		Attempt: 1, StartedAt: time.Now().UTC(),
	})
	if err != nil || !created {
		t.Fatalf("StartV1Run = (%v, %v), want new run", created, err)
	}

	// owner-b(接管执行者)持有 lease;owner-a 的迟到失败必须被拒绝。
	if acquired, err := service.AcquireRunLease(ctx, tenantID, runID, 1, "owner-b", time.Hour); err != nil || !acquired {
		t.Fatalf("acquire by owner-b = (%v, %v), want true", acquired, err)
	}
	lateFailure := &V1DurableRunCompletion{
		TenantID: tenantID, RunID: runID, Attempt: 1,
		Status: RunStatusFailed, LeaseOwner: "owner-a", FinishedAt: time.Now().UTC(),
	}
	if err := service.CompleteV1Run(ctx, lateFailure); !errors.Is(err, ErrLeaseNotHeld) {
		t.Fatalf("stale executor completion = %v, want ErrLeaseNotHeld", err)
	}
	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM agent_runs WHERE id = $1`, runID,
	).Scan(&status); err != nil || status != RunStatusRunning {
		t.Fatalf("run status after rejected late failure = (%s, %v), want running", status, err)
	}

	// 持有者 owner-b 的成功正常生效。
	if err := service.CompleteV1Run(ctx, &V1DurableRunCompletion{
		TenantID: tenantID, RunID: runID, Attempt: 1,
		Status: RunStatusSucceeded, LeaseOwner: "owner-b", FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("lease-owner completion = %v, want nil", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT status FROM agent_runs WHERE id = $1`, runID,
	).Scan(&status); err != nil || status != RunStatusSucceeded {
		t.Fatalf("run status after owner completion = (%s, %v), want succeeded", status, err)
	}

	// 终态后,迟到者的同状态完成按幂等重复处理,不再报错也不改状态。
	if err := service.CompleteV1Run(ctx, &V1DurableRunCompletion{
		TenantID: tenantID, RunID: runID, Attempt: 1,
		Status: RunStatusSucceeded, LeaseOwner: "owner-a", FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("duplicate terminal completion = %v, want nil (idempotent)", err)
	}
	var logStatus string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM agent_run_logs WHERE durable_run_id = $1`, runID,
	).Scan(&logStatus); err != nil || logStatus != RunStatusSucceeded {
		t.Fatalf("compat log status = (%s, %v), want succeeded", logStatus, err)
	}
}
