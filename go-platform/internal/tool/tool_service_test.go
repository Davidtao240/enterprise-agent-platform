package tool

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
)

func ptrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// fakeToolRepo 内存实现 Repository 供单元测试。
type fakeToolRepo struct {
	tools map[string]*Tool
}

func (f *fakeToolRepo) ListTools(ctx context.Context, domain, riskLevel, isShared, status string) ([]Tool, error) {
	var result []Tool
	for _, t := range f.tools {
		if domain != "" && t.Domain != domain {
			continue
		}
		if status != "" && t.Status != status {
			continue
		}
		result = append(result, *t)
	}
	return result, nil
}

func (f *fakeToolRepo) FindToolByIDAndVersion(ctx context.Context, toolID, version string) (*Tool, error) {
	for _, t := range f.tools {
		if t.ToolID == toolID && t.Version == version && t.Status == "active" {
			return t, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeToolRepo) FindToolByID(ctx context.Context, toolID string) (*Tool, error) {
	for _, t := range f.tools {
		if t.ToolID == toolID && t.Status == "active" {
			return t, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeToolRepo) FindPermissionsByAgent(ctx context.Context, agentID, businessAppCode string) ([]AgentToolPermission, error) {
	return nil, nil
}

// fakeToolCallRepo 内存实现 ToolCallRepository 供单元测试。
type fakeToolCallRepo struct {
	calls    map[string]*ToolCall
	byKey    map[string]*ToolCall
	failNext bool
}

func newFakeToolCallRepo() *fakeToolCallRepo {
	return &fakeToolCallRepo{
		calls: make(map[string]*ToolCall),
		byKey: make(map[string]*ToolCall),
	}
}

func (f *fakeToolCallRepo) Create(ctx context.Context, req *ToolCallRequest) (*ToolCall, error) {
	if f.failNext {
		f.failNext = false
		return nil, errors.New("db error")
	}
	tc := &ToolCall{
		ID:               "tc-" + req.IdempotencyKey,
		TenantID:         req.TenantID,
		RunID:            req.RunID,
		StepID:           &req.StepID,
		ToolID:           req.ToolID,
		ToolVersion:      req.ToolVersion,
		BusinessAppCode:  req.BusinessAppCode,
		ConnectorBindingID: ptrIfNotEmpty(req.ConnectorBindingID),
		PolicyVersion:    req.PolicyVersion,
		RiskLevel:        req.RiskLevel,
		Status:           req.Status,
		IdempotencyKey:   req.IdempotencyKey,
		InputHash:        req.InputHash,
		InputSummaryJSON: &req.InputSummaryJSON,
		TraceID:          req.TraceID,
		TimeoutAt:        req.TimeoutAt,
	}
	f.calls[tc.ID] = tc
	f.byKey[req.TenantID+":"+req.IdempotencyKey] = tc
	return tc, nil
}

func (f *fakeToolCallRepo) GetByID(ctx context.Context, id string) (*ToolCall, error) {
	tc, ok := f.calls[id]
	if !ok {
		return nil, ErrToolCallNotFound
	}
	return tc, nil
}

func (f *fakeToolCallRepo) GetByIdempotencyKey(ctx context.Context, tenantID, idempotencyKey string) (*ToolCall, error) {
	tc, ok := f.byKey[tenantID+":"+idempotencyKey]
	if !ok {
		return nil, nil
	}
	return tc, nil
}

func (f *fakeToolCallRepo) UpdateStatus(ctx context.Context, id string, status ToolCallStatus, fields map[string]any) error {
	tc, ok := f.calls[id]
	if !ok {
		return ErrToolCallNotFound
	}
	tc.Status = status
	applyFakeFields(tc, fields)
	return nil
}

func (f *fakeToolCallRepo) UpdateStatusGuarded(ctx context.Context, id string, expectedFrom, status ToolCallStatus, fields map[string]any) error {
	tc, ok := f.calls[id]
	if !ok {
		return ErrToolCallNotFound
	}
	if tc.Status != expectedFrom && !(expectedFrom == ToolCallStatusIndeterminate && status == ToolCallStatusIndeterminate) {
		return &transitionError{from: string(tc.Status), to: string(status)}
	}
	tc.Status = status
	applyFakeFields(tc, fields)
	return nil
}

// transitionError 模拟守卫失败的错误(不匹配 ErrInvalidTransition 以外语义)。
type transitionError struct {
	from string
	to   string
}

func (e *transitionError) Error() string {
	return "invalid tool_call status transition: " + e.from + " → " + e.to
}

// applyFakeFields 将 fields 写入内存 ToolCall(覆盖生命周期用到的列)。
func applyFakeFields(tc *ToolCall, fields map[string]any) {
	for k, v := range fields {
		switch k {
		case "status":
			if s, ok := v.(ToolCallStatus); ok {
				tc.Status = s
			}
		case "approval_task_id":
			if s, ok := v.(string); ok {
				tc.ApprovalTaskID = &s
			}
		case "output_summary_json":
			if s, ok := v.(string); ok {
				tc.OutputSummaryJSON = &s
			}
		case "error_json":
			switch val := v.(type) {
			case string:
				tc.ErrorJSON = &val
			case []byte:
				s := string(val)
				tc.ErrorJSON = &s
			}
		case "verification_json":
			if s, ok := v.(string); ok {
				tc.VerificationJSON = &s
			}
		case "external_request_id":
			if s, ok := v.(string); ok {
				tc.ExternalRequestID = &s
			}
		case "external_object_id":
			if s, ok := v.(string); ok {
				tc.ExternalObjectID = &s
			}
		case "retry_count":
			if n, ok := v.(int); ok {
				tc.RetryCount = n
			}
		case "is_dead_letter":
			if b, ok := v.(bool); ok {
				tc.IsDeadLetter = b
			}
		case "timeout_at":
			if t, ok := v.(*time.Time); ok && t != nil {
				tc.TimeoutAt = t
			}
		}
	}
}

func (f *fakeToolCallRepo) ListByRun(ctx context.Context, runID string) ([]*ToolCall, error) {
	var result []*ToolCall
	for _, tc := range f.calls {
		if tc.RunID == runID {
			result = append(result, tc)
		}
	}
	return result, nil
}

func (f *fakeToolCallRepo) ListDeadLetters(ctx context.Context, limit int) ([]*ToolCall, error) {
	var result []*ToolCall
	for _, tc := range f.calls {
		if tc.IsDeadLetter {
			result = append(result, tc)
		}
	}
	return result, nil
}

func (f *fakeToolCallRepo) ListByTrace(ctx context.Context, traceID string) ([]*ToolCall, error) {
	var result []*ToolCall
	for _, tc := range f.calls {
		if tc.TraceID == traceID {
			result = append(result, tc)
		}
	}
	return result, nil
}

// noopAuditLogger 审计日志空实现。
type noopAuditLogger struct{}

func (noopAuditLogger) InsertLog(ctx context.Context, entry audit.AuditLogEntry) (string, time.Time, error) {
	return "", time.Time{}, nil
}

// fakeDomainPolicyProvider 域策略假实现。
type fakeDomainPolicyProvider struct {
	allowed map[string][]string
}

func (f *fakeDomainPolicyProvider) FindAllowedDomains(ctx context.Context, businessAppCode string) ([]string, error) {
	return f.allowed[businessAppCode], nil
}

// fakeAgentPermProvider Agent 权限假实现。
type fakeAgentPermProvider struct {
	allowed map[string]bool // key: agentID:toolID:bizCode
}

func (f *fakeAgentPermProvider) HasToolPermission(ctx context.Context, agentID, toolID, businessAppCode string) (bool, error) {
	key := agentID + ":" + toolID + ":" + businessAppCode
	return f.allowed[key], nil
}

func setupTestService() (*Service, *fakeToolRepo, *fakeToolCallRepo) {
	toolRepo := &fakeToolRepo{
		tools: map[string]*Tool{
			"parse_csv": {ToolID: "parse_csv", Version: "1.0", Domain: "shared", RiskLevel: "low", Status: "active", IsShared: true},
			"archive_report": {ToolID: "archive_report", Version: "1.0", Domain: "finance", RiskLevel: "high", Status: "active", IsShared: false},
			"validate_metrics": {ToolID: "validate_metrics", Version: "2.0", Domain: "finance", RiskLevel: "medium", Status: "active", IsShared: false},
		},
	}
	callRepo := newFakeToolCallRepo()
	svc := NewService(toolRepo, callRepo, noopAuditLogger{})
	return svc, toolRepo, callRepo
}

func TestToolVersionResolution(t *testing.T) {
	svc, _, _ := setupTestService()
	ctx := context.Background()

	// 指定版本:成功
	tool, err := svc.resolveTool(ctx, "validate_metrics", "2.0")
	if err != nil {
		t.Fatalf("version resolve failed: %v", err)
	}
	if tool.Version != "2.0" {
		t.Fatalf("version = %s, want 2.0", tool.Version)
	}

	// 版本不存在
	if _, err := svc.resolveTool(ctx, "validate_metrics", "9.9"); err == nil {
		t.Fatal("expected error for nonexistent version")
	}

	// 不指定版本:取最新
	tool, err = svc.resolveTool(ctx, "parse_csv", "")
	if err != nil {
		t.Fatalf("fallback resolve failed: %v", err)
	}
	if tool.Version != "1.0" {
		t.Fatalf("fallback version = %s, want 1.0", tool.Version)
	}

	// 工具不存在
	if _, err := svc.resolveTool(ctx, "nonexistent", ""); err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}

func TestToolExecutionCrossDomainRejected(t *testing.T) {
	svc, _, _ := setupTestService()
	ctx := context.Background()
	policy := &fakeDomainPolicyProvider{allowed: map[string][]string{"finance": {"finance", "shared"}}}

	// archive_report 是 finance 域,在 finance 业务下应通过
	_, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "archive_report", ToolVersion: "1.0",
		IdempotencyKey: "key-1", ArgumentsJSON: "{}",
	}, policy, nil)
	if err != nil {
		t.Fatalf("finance domain should pass: %v", err)
	}

	// 用 finance 业务调用 HR 域工具(不在允许列表中)
	policy2 := &fakeDomainPolicyProvider{allowed: map[string][]string{"hr": {"hr", "shared"}}}
	_, err = svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r2", AgentID: "a2", BusinessAppCode: "hr",
		ToolID: "archive_report", ToolVersion: "1.0",
		IdempotencyKey: "key-2", ArgumentsJSON: "{}",
	}, policy2, nil)
	if !errors.Is(err, ErrDomainPolicyViolation) {
		t.Fatalf("cross-domain should be rejected, got: %v", err)
	}
}

func TestToolExecutionNotAllowedAgent(t *testing.T) {
	svc, _, _ := setupTestService()
	ctx := context.Background()
	perm := &fakeAgentPermProvider{allowed: map[string]bool{
		"allowed-agent:parse_csv:finance": true,
	}}

	// 有权限
	_, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "allowed-agent", BusinessAppCode: "finance",
		ToolID: "parse_csv", ToolVersion: "1.0",
		IdempotencyKey: "key-1", ArgumentsJSON: "{}",
	}, nil, perm)
	if err != nil {
		t.Fatalf("allowed agent should pass: %v", err)
	}

	// 无权限
	_, err = svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r2", AgentID: "denied-agent", BusinessAppCode: "finance",
		ToolID: "parse_csv", ToolVersion: "1.0",
		IdempotencyKey: "key-2", ArgumentsJSON: "{}",
	}, nil, perm)
	if !errors.Is(err, ErrToolNotAllowed) {
		t.Fatalf("denied agent should be rejected, got: %v", err)
	}
}

func TestToolExecutionIdempotentReplay(t *testing.T) {
	svc, _, _ := setupTestService()
	ctx := context.Background()

	result1, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "parse_csv", ToolVersion: "1.0",
		IdempotencyKey: "idem-1", ArgumentsJSON: `{"data":"test"}`,
	}, nil, nil)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if result1.ToolCallID == "" {
		t.Fatal("expected tool_call_id on first call")
	}

	// 相同幂等键:返回已有记录
	result2, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "parse_csv", ToolVersion: "1.0",
		IdempotencyKey: "idem-1", ArgumentsJSON: `{"data":"different"}`,
	}, nil, nil)
	if err != nil {
		t.Fatalf("idempotent replay failed: %v", err)
	}
	if result2.ToolCallID != result1.ToolCallID {
		t.Fatalf("replay should return same tool_call_id, got %s vs %s", result2.ToolCallID, result1.ToolCallID)
	}
}

func TestToolExecutionHighRiskApproval(t *testing.T) {
	svc, _, _ := setupTestService()
	ctx := context.Background()

	// high risk → pending_approval
	result, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "archive_report", ToolVersion: "1.0",
		IdempotencyKey: "high-risk-1", ArgumentsJSON: "{}",
	}, nil, nil)
	if err != nil {
		t.Fatalf("high risk call failed: %v", err)
	}
	if result.Status != ToolCallStatusPendingApproval {
		t.Fatalf("high risk status = %s, want pending_approval", result.Status)
	}
	if !result.ApprovalReq {
		t.Fatal("high risk should require approval")
	}

	// medium risk → executing
	result, err = svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r2", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "validate_metrics", ToolVersion: "2.0",
		IdempotencyKey: "med-risk-1", ArgumentsJSON: "{}",
	}, nil, nil)
	if err != nil {
		t.Fatalf("medium risk call failed: %v", err)
	}
	if result.Status != ToolCallStatusExecuting {
		t.Fatalf("medium risk status = %s, want executing", result.Status)
	}
}

func TestInputHashConsistency(t *testing.T) {
	h1 := computeInputHash(`{"a":1}`)
	h2 := computeInputHash(`{"a":1}`)
	h3 := computeInputHash(`{"a":2}`)
	if h1 != h2 {
		t.Fatal("same input should produce same hash")
	}
	if h1 == h3 {
		t.Fatal("different input should produce different hash")
	}
	if len(h1) != 64 {
		t.Fatalf("hash length = %d, want 64 (sha256 hex)", len(h1))
	}
}
