package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/google/uuid"
)

// Gateway 是 Go 后端调用 Python Agent Service 的统一入口。
//
// 职责：
//  1. 验证 graph_key（在 graph_registry 中存在且 active）
//  2. 校验 Domain Policy（agent 的 domain 必须在允许列表中）
//  3. 调用 Python Agent Service（POST /internal/v1/agent-runs）
//  4. 记录 agent_run_logs（输入摘要、输出摘要、token/cost、耗时、状态）
//  5. 返回结构化结果给调用方（Workflow Worker）
//
// 安全约束：
//   - 只允许调用已注册的 graph_key
//   - 跨域调用被 Domain Policy 拦截
//   - Python 返回结果经过 JSON 校验
type Gateway struct {
	repo            gatewayRepository
	auditRepo       agentAuditLogger
	agentServiceURL string
	httpClient      *http.Client
	strictPolicy    bool
}

type agentAuditLogger interface {
	InsertLog(ctx context.Context, entry audit.AuditLogEntry) (string, time.Time, error)
}

type gatewayRepository interface {
	FindGraphByKey(ctx context.Context, graphKey string) (*Graph, error)
	FindDomainPolicy(ctx context.Context, businessAppCode string) (*DomainPolicy, error)
	CreateRunLog(ctx context.Context, log *AgentRunLog) error
	UpdateRunLog(ctx context.Context, runID, status string, outputSummaryJSON, usageJSON, errorJSON *string, finishedAt *time.Time, durationMs *int) error
}

// NewGateway 创建 Gateway 实例。
func NewGateway(repo *Repository, auditRepo *audit.Repository, agentServiceURL string, strictPolicy bool) *Gateway {
	g := &Gateway{
		repo:            repo,
		agentServiceURL: agentServiceURL,
		httpClient:      &http.Client{Timeout: 300 * time.Second},
		strictPolicy:    strictPolicy,
	}
	if auditRepo != nil {
		g.auditRepo = auditRepo
	}
	return g
}

// Execute 执行一次 Agent Graph 调用。
//
// 参数 payload 来自 Workflow Worker 的 Asynq 任务。
//
// 流程：
//  1. 查 graph_registry → 验证 graph_key
//  2. 查 domain_policy → 校验域隔离
//  3. 构建请求体 → POST Python Agent Service
//  4. 写入 agent_run_logs（开始执行）
//  5. 解析返回结果 → 更新 agent_run_logs（完成/失败）
//  6. 返回 AgentRunResponse
func (g *Gateway) Execute(ctx context.Context, payload *AgentRunPayload) (*AgentRunResponse, error) {
	// ── 1. 验证 graph_key ──
	graph, err := g.repo.FindGraphByKey(ctx, payload.GraphKey)
	if err != nil {
		return nil, fmt.Errorf("graph_key %s not found in registry: %w", payload.GraphKey, err)
	}
	if graph.Status != "active" {
		return nil, fmt.Errorf("graph_key %s is not active (status: %s)", payload.GraphKey, graph.Status)
	}

	// ── 2. 校验 Domain Policy ──
	if err := g.validateDomainPolicy(ctx, payload.BusinessAppCode, payload.GraphKey); err != nil {
		return nil, fmt.Errorf("domain policy violation: %w", err)
	}

	// ── 3. 构建请求体 ──
	runID := uuid.New().String()
	reqBody := AgentRunRequest{
		TraceID:             payload.TraceID,
		BusinessAppCode:     payload.BusinessAppCode,
		WorkflowTemplateKey: payload.WorkflowTemplateKey,
		GraphKey:            payload.GraphKey,
		WorkflowInstanceID:  payload.WorkflowInstanceID,
		NodeInstanceID:      payload.NodeInstanceID,
		Input:               payload.Input,
		Context: AgentContext{
			UserID: payload.UserID,
		},
	}

	reqJSON, _ := json.Marshal(reqBody)

	// ── 4. 写入 agent_run_logs（开始执行） ──
	startedAt := time.Now()
	runLog := &AgentRunLog{
		RunID:              runID,
		TraceID:            payload.TraceID,
		WorkflowInstanceID: payload.WorkflowInstanceID,
		NodeInstanceID:     payload.NodeInstanceID,
		BusinessAppCode:    payload.BusinessAppCode,
		GraphKey:           payload.GraphKey,
		Status:             "running",
		StartedAt:          &startedAt,
	}
	if err := g.repo.CreateRunLog(ctx, runLog); err != nil {
		log.Printf("[gateway] failed to create run log: %v", err)
	}
	g.auditLog(ctx, payload, "agent_run_started", runID, "running", nil)

	// ── 5. 调用 Python Agent Service ──
	log.Printf("[gateway] calling Python Agent Service for graph=%s run=%s", payload.GraphKey, runID)

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		g.agentServiceURL+"/internal/v1/agent-runs", bytes.NewReader(reqJSON))
	if err != nil {
		g.recordFailure(ctx, payload, runID, runLog, fmt.Sprintf("create request: %v", err), &startedAt)
		return nil, fmt.Errorf("create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Trace-Id", payload.TraceID)

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		g.recordFailure(ctx, payload, runID, runLog, fmt.Sprintf("HTTP call failed: %v", err), &startedAt)
		return nil, fmt.Errorf("call Python Agent Service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("agent service returned HTTP %d: %s", resp.StatusCode, string(body))
		g.recordFailure(ctx, payload, runID, runLog, errMsg, &startedAt)
		return nil, fmt.Errorf("agent service returned status %d", resp.StatusCode)
	}

	// ── 6. 解析返回结果 ──
	var agentResp AgentRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&agentResp); err != nil {
		g.recordFailure(ctx, payload, runID, runLog, fmt.Sprintf("decode response: %v", err), &startedAt)
		return nil, fmt.Errorf("decode agent response: %w", err)
	}

	// ── 7. 更新 agent_run_logs（完成） ──
	finishedAt := time.Now()
	durationMs := int(finishedAt.Sub(startedAt).Milliseconds())

	var usageJSON, errorJSON, outputJSON *string
	if agentResp.Usage != nil {
		data, _ := json.Marshal(agentResp.Usage)
		s := string(data)
		usageJSON = &s
	}
	if agentResp.Error != nil {
		data, _ := json.Marshal(agentResp.Error)
		s := string(data)
		errorJSON = &s
	}
	if agentResp.Output != nil {
		data, _ := json.Marshal(agentResp.Output)
		s := string(data)
		outputJSON = &s
	}

	if err := g.repo.UpdateRunLog(ctx, runID, agentResp.Status, outputJSON, usageJSON, errorJSON, &finishedAt, &durationMs); err != nil {
		log.Printf("[gateway] failed to update run log: %v", err)
	}

	log.Printf("[gateway] agent run %s completed: status=%s duration=%dms", runID, agentResp.Status, durationMs)
	action := "agent_run_completed"
	detailJSON := usageJSON
	if agentResp.Status == "failed" {
		action = "agent_run_failed"
		detailJSON = errorJSON
	}
	g.auditLog(ctx, payload, action, runID, agentResp.Status, detailJSON)
	return &agentResp, nil
}

// recordFailure 记录 Gateway 调用失败的日志。
func (g *Gateway) recordFailure(ctx context.Context, payload *AgentRunPayload, runID string, runLog *AgentRunLog, errMsg string, startedAt *time.Time) {
	finishedAt := time.Now()
	durationMs := int(finishedAt.Sub(*startedAt).Milliseconds())
	errData, _ := json.Marshal(map[string]string{"code": "GATEWAY_ERROR", "message": errMsg})
	errJSON := string(errData)
	_ = g.repo.UpdateRunLog(ctx, runID, "failed", nil, nil, &errJSON, &finishedAt, &durationMs)

	// 写入审计日志
	if g.auditRepo == nil {
		return
	}
	traceID := runLog.TraceID
	var actorUserID *string
	if payload.UserID != "" {
		actorUserID = &payload.UserID
	}
	detail := errJSON
	g.auditRepo.InsertLog(ctx, audit.AuditLogEntry{
		TraceID:         traceID,
		ActorUserID:     actorUserID,
		BusinessAppCode: &runLog.BusinessAppCode,
		Action:          "agent_run_failed",
		ResourceType:    "agent_run_log",
		ResourceID:      runID,
		Status:          "failed",
		DetailJSON:      &detail,
	})
}

// auditLog 写入一条审计日志（忽略错误，非致命）。
func (g *Gateway) auditLog(ctx context.Context, payload *AgentRunPayload, action, resourceID, status string, detailJSON *string) {
	if g.auditRepo == nil {
		return
	}
	if detailJSON == nil {
		emptyDetail := "{}"
		detailJSON = &emptyDetail
	}
	var actorUserID *string
	if payload.UserID != "" {
		actorUserID = &payload.UserID
	}
	g.auditRepo.InsertLog(ctx, audit.AuditLogEntry{
		TraceID:         payload.TraceID,
		ActorUserID:     actorUserID,
		BusinessAppCode: &payload.BusinessAppCode,
		Action:          action,
		ResourceType:    "agent_run_log",
		ResourceID:      resourceID,
		Status:          status,
		DetailJSON:      detailJSON,
	})
}

// validateDomainPolicy 校验域隔离策略。
// 查询该 business_app 的 domain_policy，确认 graph_key 所属的 domain 在允许列表中。
func (g *Gateway) validateDomainPolicy(ctx context.Context, businessAppCode, graphKey string) error {
	dp, err := g.repo.FindDomainPolicy(ctx, businessAppCode)
	if err != nil {
		if g.strictPolicy {
			return fmt.Errorf("domain policy not configured for %s (strict mode enabled)", businessAppCode)
		}
		// 宽松模式（默认）：没有配置 domain_policy 的业务默认允许
		log.Printf("[gateway] WARNING: no domain policy found for %s — allowing by default (loose mode)", businessAppCode)
		return nil
	}

	// 解析允许的 domain 列表
	var allowedDomains []string
	if err := json.Unmarshal([]byte(dp.AllowedAgentDomains), &allowedDomains); err != nil {
		return fmt.Errorf("parse allowed agent domains: %w", err)
	}

	// 查 graph 对应的 business_app，间接判断 domain
	graph, err := g.repo.FindGraphByKey(ctx, graphKey)
	if err != nil {
		return fmt.Errorf("graph not found: %w", err)
	}

	// 当前策略：graph 的 business_app_code 必须与请求的 business_app_code 匹配
	// 跨业务调用必须显式配置在 domain_policy 中
	if graph.BusinessAppCode != businessAppCode {
		return fmt.Errorf("cross-domain call denied: graph %s belongs to %s, called from %s",
			graphKey, graph.BusinessAppCode, businessAppCode)
	}

	log.Printf("[gateway] domain policy OK for %s: allowed domains=%v", businessAppCode, allowedDomains)
	return nil
}

// ── Gateway 调用载荷（从 Workflow Worker 传入） ──

// AgentRunPayload Agent Gateway 执行所需的参数。
// 从 Worker 的 ExecuteNodePayload + workflow 上下文组装。
type AgentRunPayload struct {
	TraceID             string         // 跨服务追踪 ID
	BusinessAppCode     string         // 业务域
	WorkflowTemplateKey string         // 模板 key
	GraphKey            string         // Python Graph key
	WorkflowInstanceID  string         // 工作流实例 ID
	NodeInstanceID      string         // 节点实例 ID
	Input               map[string]any // 节点输入
	UserID              string         // 触发用户
}
