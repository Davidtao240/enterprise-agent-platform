package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
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
	repo      gatewayRepository
	durable   durableRunLifecycle
	durableV2 interface {
		StartV2Run(context.Context, *V2DurableRunStart) (*DurableRun, bool, error)
	}
	runtimeV2          *RuntimeV2Client
	runtimeToken       string
	auditRepo          agentAuditLogger
	agentServiceURL    string
	httpClient         *http.Client
	strictPolicy       bool
	instanceID         string
	leaseTTL           time.Duration
	heartbeatInterval  time.Duration
	modelConfigVersion string
	workerV2           bool
	experiments        ExperimentRouter // M5-C: 流量切分单一收口(Canary/Shadow)
}

// ExperimentRoute M5-C 流量切分决策结果。
type ExperimentRoute struct {
	// ResolvedGraphKey 实际应执行的 graph_key(Canary 命中时为候选版本,
	// 否则与请求一致)。
	ResolvedGraphKey string
	// CanaryReleaseID 命中的 Canary 发布 ID(非空表示命中候选版本)。
	CanaryReleaseID string
	// ShadowGraphKey 命中影子取样时的影子执行目标(空表示未命中)。
	ShadowGraphKey string
	// ShadowRuleID 命中的 Shadow 规则 ID。
	ShadowRuleID string
}

// ExperimentRouter M5-C 实验路由决策(由 internal/experiment 实现;
// 消费侧接口,agent 包不依赖 experiment 包,避免循环引用)。
type ExperimentRouter interface {
	// ResolveRoute 按 hash(run_id)%100 对 Canary 阶梯与 Shadow 规则取样。
	ResolveRoute(ctx context.Context, tenantID, businessAppCode, graphKey, runID string) (ExperimentRoute, error)
	// RecordShadowExecution 记录一次影子执行(主 Run ↔ 影子 Run 关联)。
	RecordShadowExecution(ctx context.Context, tenantID, ruleID, primaryRunID, shadowRunID string) error
}

// SetExperimentRouter 注入 M5-C 实验路由(nil 表示关闭实验分流)。
func (g *Gateway) SetExperimentRouter(router ExperimentRouter) {
	g.experiments = router
}

// resolveExperimentGraph M5-C 流量切分共用决策(V1 Execute 与 V2 StartV2
// 单一收口; Spec §4.6):
//  1. 解析基线 graph(任何情况下都必须存在);
//  2. Router 按 hash(run_id) 取样,Canary 命中时重定向到候选版本;
//  3. 候选版本必须 active、属于同一业务域且通过 Domain Policy。
//
// Shadow 取样结果随 route 返回,由调用方在 Run 创建成功后 best-effort 复制。
func (g *Gateway) resolveExperimentGraph(ctx context.Context, payload *AgentRunPayload, runID string) (ExperimentRoute, *Graph, error) {
	route := ExperimentRoute{ResolvedGraphKey: payload.GraphKey}
	graph, err := g.repo.FindGraphByKey(ctx, payload.GraphKey)
	if err != nil {
		return route, nil, fmt.Errorf("graph_key %s not found in registry: %w", payload.GraphKey, err)
	}
	if g.experiments == nil {
		return route, graph, nil
	}
	route, err = g.experiments.ResolveRoute(ctx, payload.TenantID, payload.BusinessAppCode, payload.GraphKey, runID)
	if err != nil {
		return route, graph, fmt.Errorf("experiment route: %w", err)
	}
	if route.ResolvedGraphKey != "" && route.ResolvedGraphKey != payload.GraphKey {
		candidate, lookupErr := g.repo.FindGraphByKey(ctx, route.ResolvedGraphKey)
		if lookupErr != nil {
			return route, graph, fmt.Errorf("canary candidate graph_key %s not found: %w", route.ResolvedGraphKey, lookupErr)
		}
		if candidate.Status != "active" || candidate.BusinessAppCode != payload.BusinessAppCode {
			return route, graph, fmt.Errorf("canary candidate graph_key %s is not active for business app %s", route.ResolvedGraphKey, payload.BusinessAppCode)
		}
		if policyErr := g.validateDomainPolicy(ctx, payload.BusinessAppCode, route.ResolvedGraphKey); policyErr != nil {
			return route, graph, fmt.Errorf("canary candidate domain policy violation: %w", policyErr)
		}
		graph = candidate
	}
	return route, graph, nil
}

// M1-C-A lease 默认参数:TTL 必须明显大于心跳周期,保证存活 Worker 的
// heartbeat 能持续续租;Worker 失联后 lease 在 TTL 内过期,收敛扫描器接管。
const (
	defaultLeaseTTL          = 90 * time.Second
	defaultHeartbeatInterval = 30 * time.Second
)

type agentAuditLogger interface {
	InsertLog(ctx context.Context, entry audit.AuditLogEntry) (string, time.Time, error)
}

type gatewayRepository interface {
	FindGraphByKey(ctx context.Context, graphKey string) (*Graph, error)
	FindDomainPolicy(ctx context.Context, businessAppCode string) (*DomainPolicy, error)
}

// NewGateway 创建 Gateway 实例。
func NewGateway(repo *Repository, auditRepo *audit.Repository, agentServiceURL string, strictPolicy bool) *Gateway {
	g := &Gateway{
		repo:              repo,
		agentServiceURL:   agentServiceURL,
		httpClient:        &http.Client{Timeout: 300 * time.Second},
		strictPolicy:      strictPolicy,
		instanceID:        uuid.NewString(),
		leaseTTL:          defaultLeaseTTL,
		heartbeatInterval: defaultHeartbeatInterval,
	}
	if repo != nil {
		durable := NewDurableRunService(repo)
		g.durable = durable
		g.durableV2 = durable
	}
	if auditRepo != nil {
		g.auditRepo = auditRepo
	}
	return g
}

// ConfigureRuntimeV2 enables the additive asynchronous Runtime V2 path. The
// existing Execute method intentionally remains the Finance V1 compatibility
// bridge until Workflow completion is event-driven.
func (g *Gateway) ConfigureRuntimeV2(serviceToken string) {
	g.runtimeV2 = NewRuntimeV2Client(g.agentServiceURL, serviceToken)
	g.runtimeToken = serviceToken
}

// EnableWorkerRuntimeV2 切换 Workflow Worker 的 agent_graph 执行路径到
// Runtime V2 异步(M2-A):Start 返回后节点保持 running,终态/中断事件经
// RunEventBridge 推进。默认关闭时继续走 V1 同步桥。
func (g *Gateway) EnableWorkerRuntimeV2() {
	g.workerV2 = true
}

// SetHTTPTimeout 允许在构造后设置 HTTP client 超时。
// 生产部署通过此方法将配置文件中的超时注入 Gateway。
func (g *Gateway) SetHTTPTimeout(d time.Duration) {
	g.httpClient = &http.Client{Timeout: d}
}

// WorkerRuntimeV2Enabled 报告 Worker 是否应走 Runtime V2 异步路径。
func (g *Gateway) WorkerRuntimeV2Enabled() bool {
	return g.workerV2 && g.runtimeV2 != nil && g.durableV2 != nil
}

// RuntimeV2Client exposes the configured Runtime V2 client for the control plane.
func (g *Gateway) RuntimeV2Client() *RuntimeV2Client {
	return g.runtimeV2
}

// SetLeaseConfig 调整 lease TTL 与心跳周期(测试与调优用)。
func (g *Gateway) SetLeaseConfig(ttl, heartbeatInterval time.Duration) {
	if ttl > 0 {
		g.leaseTTL = ttl
	}
	if heartbeatInterval > 0 {
		g.heartbeatInterval = heartbeatInterval
	}
}

// SetModelConfigVersion 设置 Runtime V2 启动请求的默认模型配置版本。
func (g *Gateway) SetModelConfigVersion(version string) {
	if version != "" {
		g.modelConfigVersion = version
	}
}

// newLeaseOwner 为每次执行尝试生成唯一 owner token。
// 同进程并发的重复投递因 owner 不同无法互相穿透 lease。
func (g *Gateway) newLeaseOwner(attempt int) string {
	return fmt.Sprintf("%s:%d:%s", g.instanceID, attempt, uuid.NewString())
}

// startLeaseHeartbeat 在 Python 调用期间周期续租。lease 被接管(owner 不匹配)
// 时停止心跳并告警;HTTP 调用本身不被中断,迟到完成由状态幂等收口。
// 每次心跳使用独立超时,防止数据库驱动不支持 context 取消导致 goroutine 泄漏。
func (g *Gateway) startLeaseHeartbeat(ctx context.Context, tenantID, runID string, attempt int, owner string) func() {
	hbCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if g.heartbeatInterval <= 0 {
			return
		}
		ticker := time.NewTicker(g.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				// 每次心跳独立超时,防止单次数据库阻塞导致 goroutine 泄漏。
				heartbeatTimeout := g.heartbeatInterval
				if heartbeatTimeout < 5*time.Second {
					heartbeatTimeout = 5 * time.Second
				}
				timeoutCtx, timeoutCancel := context.WithTimeout(hbCtx, heartbeatTimeout)
				ok, err := g.durable.HeartbeatRunLease(timeoutCtx, tenantID, runID, attempt, owner, g.leaseTTL)
				timeoutCancel()
				if err != nil {
					log.Printf("[gateway] heartbeat durable run %s failed: %v", runID, err)
					continue
				}
				if !ok {
					log.Printf("[gateway] lease for durable run %s was taken over; stopping heartbeat", runID)
					return
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func isTerminalRunStatus(status string) bool {
	switch status {
	case RunStatusSucceeded, RunStatusFailed, RunStatusCancelled:
		return true
	default:
		return false
	}
}

// lateResultReplay 在迟到结果被拒绝(ErrLeaseNotHeld)时回放 Run 当前状态,
// 而不是向 Worker 报错:非终态回放使 Worker 跳过推进(接管者仍在执行),
// 终态回放由既有幂等路径收敛节点。读取失败时退化为最小 running 回放,
// 交给收敛扫描器对账,绝不让迟到结果回退节点。
func (g *Gateway) lateResultReplay(ctx context.Context, tenantID, runID string) (*AgentRunResponse, error) {
	if run, err := g.durable.FindRun(ctx, tenantID, runID); err == nil && run != nil {
		return replayDurableRun(run), nil
	}
	return &AgentRunResponse{RunID: runID, Status: RunStatusRunning, Output: map[string]any{}, Replayed: true}, nil
}

// finalizeAttemptError 统一失败收口:默认持久化失败终态并返回原错误;若失败
// 写入因 lease 已被接管而被拒绝(迟到结果),回放当前 Run 状态且不报错,
// 避免 Worker 触发 OnNodeFailed 回退接管者正在执行的节点。
func (g *Gateway) finalizeAttemptError(
	ctx context.Context, payload *AgentRunPayload, runID string, attempt int,
	owner, errMsg string, startedAt time.Time, cause error,
) (*AgentRunResponse, error) {
	persistErr := g.recordFailure(ctx, payload, runID, attempt, owner, errMsg, startedAt)
	if errors.Is(persistErr, ErrLeaseNotHeld) {
		log.Printf("[gateway] late failure for durable run %s rejected (lease taken over); replaying current state", runID)
		return g.lateResultReplay(ctx, payload.TenantID, runID)
	}
	if persistErr != nil {
		return nil, fmt.Errorf("%w; persist failure: %v", cause, persistErr)
	}
	return nil, cause
}

// StartV2 persists the Go Run index before sending an idempotent asynchronous
// Start command to Python. It is not used by the V1 Workflow Worker yet.
func (g *Gateway) StartV2(
	ctx context.Context,
	payload *AgentRunPayload,
	configuration RuntimeV2Configuration,
	budget RuntimeV2Budget,
) (*RuntimeV2AcceptedResponse, error) {
	if payload == nil {
		return nil, fmt.Errorf("Runtime V2 payload is required")
	}
	if g.runtimeV2 == nil || g.durableV2 == nil {
		return nil, fmt.Errorf("Runtime V2 gateway is not configured")
	}
	runID := uuid.NewString()
	// ── M5-C: 实验路由单一收口(Canary 切流 + Shadow 复制取样) ──
	route, graph, err := g.resolveExperimentGraph(ctx, payload, runID)
	if err != nil {
		return nil, err
	}
	if graph.Status != "active" || graph.BusinessAppCode != payload.BusinessAppCode {
		return nil, fmt.Errorf("graph_key %s is not active for business app %s", graph.GraphKey, payload.BusinessAppCode)
	}
	if err := g.validateDomainPolicy(ctx, payload.BusinessAppCode, graph.GraphKey); err != nil {
		return nil, fmt.Errorf("domain policy violation: %w", err)
	}
	// 未显式提供的版本,从 registry/template 与模型配置推导(域中立)。
	if configuration.AgentDefinitionVersion == "" {
		configuration.AgentDefinitionVersion = graph.Version
	}
	if configuration.ProfileOrSkillVersion == "" {
		configuration.ProfileOrSkillVersion = payload.WorkflowTemplateVersion
	}
	if configuration.ModelConfigVersion == "" {
		configuration.ModelConfigVersion = g.modelConfigVersion
		if configuration.ModelConfigVersion == "" {
			configuration.ModelConfigVersion = "model:default"
		}
	}
	if configuration.AgentDefinitionVersion == "" || configuration.ProfileOrSkillVersion == "" || configuration.ModelConfigVersion == "" {
		return nil, fmt.Errorf("Runtime V2 immutable configuration versions are required")
	}
	if budget.MaxSteps <= 0 {
		budget.MaxSteps = 30
	}
	attempt := payload.Attempt
	if attempt <= 0 {
		attempt = 1
	}
	// M5-C: Canary 命中时在 Run metadata 记录发布 ID(可观测/回滚统计)。
	var metadataString *string
	if route.CanaryReleaseID != "" {
		meta, marshalErr := json.Marshal(map[string]any{"canary_release_id": route.CanaryReleaseID})
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal experiment metadata: %w", marshalErr)
		}
		s := string(meta)
		metadataString = &s
	}
	configurationSnapshot, err := json.Marshal(map[string]any{
		"protocol_version": "2.0",
		"graph":            RuntimeV2GraphIdentity{Key: graph.GraphKey, Version: graph.Version},
		"configuration":    configuration,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal Runtime V2 configuration snapshot: %w", err)
	}
	budgetJSON, err := json.Marshal(budget)
	if err != nil {
		return nil, fmt.Errorf("marshal Runtime V2 budget: %w", err)
	}
	budgetString := string(budgetJSON)
	run, created, err := g.durableV2.StartV2Run(ctx, &V2DurableRunStart{
		RunID: runID, TenantID: payload.TenantID, CreatedBy: payload.UserID,
		BusinessAppCode: payload.BusinessAppCode, WorkflowInstanceID: payload.WorkflowInstanceID,
		NodeInstanceID: payload.NodeInstanceID, ThreadTitle: payload.ThreadTitle,
		TraceID: payload.TraceID, GraphKey: graph.GraphKey, GraphVersion: graph.Version,
		ConfigurationSnapshotJSON: string(configurationSnapshot), BudgetJSON: &budgetString, Attempt: attempt,
		MetadataJSON: metadataString,
	})
	if err != nil {
		return nil, fmt.Errorf("create Runtime V2 Run index: %w", err)
	}
	runtimeConfiguration := configuration
	runtimeBudget := budget
	if !created {
		var snapshot struct {
			Configuration RuntimeV2Configuration `json:"configuration"`
		}
		if err := json.Unmarshal([]byte(run.ConfigurationSnapshotJSON), &snapshot); err != nil {
			return nil, fmt.Errorf("load immutable Runtime V2 configuration: %w", err)
		}
		runtimeConfiguration = snapshot.Configuration
		if run.BudgetJSON != nil {
			if err := json.Unmarshal([]byte(*run.BudgetJSON), &runtimeBudget); err != nil {
				return nil, fmt.Errorf("load immutable Runtime V2 budget: %w", err)
			}
		}
	}
	request := &RuntimeV2StartRequest{
		ProtocolVersion: "2.0", RunID: run.ID, ThreadID: run.ThreadID, TraceID: payload.TraceID,
		WorkflowInstanceID: payload.WorkflowInstanceID, NodeInstanceID: payload.NodeInstanceID,
		BusinessAppCode: payload.BusinessAppCode,
		Graph:           RuntimeV2GraphIdentity{Key: run.GraphKey, Version: run.GraphVersion},
		Configuration:   runtimeConfiguration, Input: payload.Input,
		TrustedContext: RuntimeV2TrustedContext{UserID: payload.UserID, TenantID: payload.TenantID},
		Budget:         runtimeBudget, Attempt: run.Attempt, IdempotencyKey: "start:" + run.ID,
	}
	accepted, err := g.runtimeV2.Start(ctx, request)
	if err != nil {
		return nil, err
	}
	if accepted.RunID != run.ID {
		return nil, fmt.Errorf("Runtime V2 returned mismatched run_id %s", accepted.RunID)
	}
	// M5-C: 命中影子取样时,best-effort 异步复制一份流量到影子版本。
	if route.ShadowGraphKey != "" {
		g.forkShadowRun(payload, route, run.ID)
	}
	return accepted, nil
}

// Execute 执行一次 Agent Graph 调用。
//
// 参数 payload 来自 Workflow Worker 的 Asynq 任务。
//
// 流程：
//  1. 查 graph_registry → 验证 graph_key
//  2. 查 domain_policy → 校验域隔离
//  3. 事务化创建 Durable Run + V1 agent_run_logs 兼容摘要
//  4. 构建请求体 → POST Python Agent Service
//  5. 解析返回结果 → 事务化完成 Run/Step/Event + 兼容摘要
//  6. 返回 AgentRunResponse
func (g *Gateway) Execute(ctx context.Context, payload *AgentRunPayload) (*AgentRunResponse, error) {
	if g.durable == nil {
		return nil, fmt.Errorf("durable run service is not configured")
	}

	// ── 1/3. 验证 graph_key + Domain Policy + M5-C 实验路由(单一收口) ──
	// run_id 先于路由决策生成:Canary/Shadow 均按将创建的 Run 身份确定性取样,
	// 重复投递复用既有 Run 时不再进入本分支(天然幂等)。
	runID := uuid.New().String()
	route, graph, err := g.resolveExperimentGraph(ctx, payload, runID)
	if err != nil {
		return nil, err
	}
	if graph.Status != "active" {
		return nil, fmt.Errorf("graph_key %s is not active (status: %s)", graph.GraphKey, graph.Status)
	}
	if err := g.validateDomainPolicy(ctx, payload.BusinessAppCode, graph.GraphKey); err != nil {
		return nil, fmt.Errorf("domain policy violation: %w", err)
	}

	attempt := payload.Attempt
	if attempt <= 0 {
		attempt = 1 // 兼容升级前已进入队列、尚未携带 attempt 的任务
	}
	startedAt := time.Now()

	inputJSON := summarizeAgentInput(payload.Input)
	configurationData, err := json.Marshal(map[string]any{
		"protocol_version":     "1.0",
		"compatibility_bridge": "/internal/v1/agent-runs",
		"workflow_template": map[string]string{
			"key":     payload.WorkflowTemplateKey,
			"version": payload.WorkflowTemplateVersion,
		},
		"graph": map[string]string{
			"key":     graph.GraphKey,
			"version": graph.Version,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal durable configuration snapshot: %w", err)
	}

	// M5-C: V1 路径 Canary 命中时记录发布 ID(与 V2 StartV2 对齐,
	// 供 CanaryCandidateStats 按 metadata 过滤候选版本流量)。
	var v1Metadata *string
	if route.CanaryReleaseID != "" {
		meta, marshalErr := json.Marshal(map[string]any{"canary_release_id": route.CanaryReleaseID})
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal experiment metadata: %w", marshalErr)
		}
		s := string(meta)
		v1Metadata = &s
	}

	run, created, err := g.durable.StartV1Run(ctx, &V1DurableRunStart{
		RunID:                     runID,
		TenantID:                  payload.TenantID,
		CreatedBy:                 payload.UserID,
		BusinessAppCode:           payload.BusinessAppCode,
		WorkflowInstanceID:        payload.WorkflowInstanceID,
		NodeInstanceID:            payload.NodeInstanceID,
		ThreadTitle:               payload.ThreadTitle,
		TraceID:                   payload.TraceID,
		GraphKey:                  graph.GraphKey,
		GraphVersion:              graph.Version,
		ConfigurationSnapshotJSON: string(configurationData),
		Attempt:                   attempt,
		InputSummaryJSON:          &inputJSON,
		StartedAt:                 startedAt,
		MetadataJSON:              v1Metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("start durable V1 run: %w", err)
	}
	owner := g.newLeaseOwner(attempt)
	if !created {
		// 终态:回放结果,由 Worker 收敛节点状态。
		if isTerminalRunStatus(run.Status) {
			log.Printf("[gateway] reusing durable run %s for node=%s attempt=%d status=%s",
				run.ID, payload.NodeInstanceID, attempt, run.Status)
			return replayDurableRun(run), nil
		}
		// 非终态:以 lease 作为存活信号。抢到 lease 说明前次执行已失联
		// (无主或已过期),用同一 Run 身份重驱动;抢不到说明持有者仍在
		// 执行,跳过以保护幂等。
		acquired, acquireErr := g.durable.AcquireRunLease(ctx, payload.TenantID, run.ID, run.Attempt, owner, g.leaseTTL)
		if acquireErr != nil {
			return nil, fmt.Errorf("acquire durable run lease: %w", acquireErr)
		}
		if !acquired {
			log.Printf("[gateway] reusing durable run %s for node=%s attempt=%d: lease held by live executor",
				run.ID, payload.NodeInstanceID, attempt)
			return replayDurableRun(run), nil
		}
		// 失联接管:用同一 Run 身份重驱动 Python。Python 侧 Graph 以 run_id
		// 为 checkpoint thread,重驱动是幂等的(已完成则返回最终状态,
		// 未完成则从 Checkpoint 恢复)。
		log.Printf("[gateway] took over lease for durable run %s (node=%s attempt=%d status=%s) and re-driving Python",
			run.ID, payload.NodeInstanceID, attempt, run.Status)
		runID = run.ID
		attempt = run.Attempt
		startedAt = *run.StartedAt
	} else {
		runID = run.ID
		// 新建 Run 理论上是无主的;若已被重复投递抢占,不阻断本次执行
		// (同一 checkpoint thread,Python 侧串行化并幂等完成)。
		if acquired, acquireErr := g.durable.AcquireRunLease(ctx, payload.TenantID, runID, attempt, owner, g.leaseTTL); acquireErr != nil {
			log.Printf("[gateway] acquire lease for new durable run %s failed: %v", runID, acquireErr)
		} else if !acquired {
			log.Printf("[gateway] lease for new durable run %s already held by another executor; continuing without lease", runID)
		}
		g.auditLog(ctx, payload, "agent_run_started", runID, RunStatusRunning, nil)
	}

	// ── M5-C: 命中影子取样时 best-effort 异步复制(仅新建 Run,重复投递不复制) ──
	if created && route.ShadowGraphKey != "" {
		g.forkShadowRun(payload, route, runID)
	}

	// ── 4. 构建兼容 V1 请求体 ──
	// GraphKey 使用路由解析后的版本(Canary 命中时为候选 graph)。
	reqBody := AgentRunRequest{
		RunID:               runID,
		TraceID:             payload.TraceID,
		BusinessAppCode:     payload.BusinessAppCode,
		WorkflowTemplateKey: payload.WorkflowTemplateKey,
		GraphKey:            graph.GraphKey,
		WorkflowInstanceID:  payload.WorkflowInstanceID,
		NodeInstanceID:      payload.NodeInstanceID,
		Input:               payload.Input,
		Context: AgentContext{
			UserID:   payload.UserID,
			TenantID: payload.TenantID,
		},
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return g.finalizeAttemptError(ctx, payload, runID, attempt, owner,
			fmt.Sprintf("marshal request: %v", err), startedAt, fmt.Errorf("marshal agent request: %w", err))
	}

	// ── 5. 调用 Python Agent Service ──
	// 调用期间周期续租;返回后无论成败停止心跳。
	stopHeartbeat := g.startLeaseHeartbeat(ctx, payload.TenantID, runID, attempt, owner)
	defer stopHeartbeat()
	log.Printf("[gateway] calling Python Agent Service for graph=%s run=%s", payload.GraphKey, runID)

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		g.agentServiceURL+"/internal/v1/agent-runs", bytes.NewReader(reqJSON))
	if err != nil {
		return g.finalizeAttemptError(ctx, payload, runID, attempt, owner,
			fmt.Sprintf("create request: %v", err), startedAt, fmt.Errorf("create HTTP request: %w", err))
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Trace-Id", payload.TraceID)
	if g.runtimeToken != "" {
		httpReq.Header.Set("X-Internal-Service-Token", g.runtimeToken)
	}

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return g.finalizeAttemptError(ctx, payload, runID, attempt, owner,
			fmt.Sprintf("HTTP call failed: %v", err), startedAt, fmt.Errorf("call Python Agent Service: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("agent service returned HTTP %d: %s", resp.StatusCode, string(body))
		return g.finalizeAttemptError(ctx, payload, runID, attempt, owner,
			errMsg, startedAt, fmt.Errorf("agent service returned status %d", resp.StatusCode))
	}

	// ── 6. 解析返回结果 ──
	var agentResp AgentRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&agentResp); err != nil {
		return g.finalizeAttemptError(ctx, payload, runID, attempt, owner,
			fmt.Sprintf("decode response: %v", err), startedAt, fmt.Errorf("decode agent response: %w", err))
	}
	agentResp.RunID = runID
	agentResp.GraphKey = graph.GraphKey
	if agentResp.Status != RunStatusSucceeded && agentResp.Status != RunStatusFailed {
		err := fmt.Errorf("invalid V1 agent response status %q", agentResp.Status)
		return g.finalizeAttemptError(ctx, payload, runID, attempt, owner,
			err.Error(), startedAt, err)
	}

	// ── 6. 原子完成 Durable Run 与 V1 兼容摘要 ──
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

	if err := g.durable.CompleteV1Run(ctx, &V1DurableRunCompletion{
		TenantID: payload.TenantID,
		RunID:    runID,
		Attempt:  attempt,
		Status:   agentResp.Status,
		// 迟到结果拒绝(M1-C-B):lease 已被接管时本次成功结果作废。
		LeaseOwner:        owner,
		OutputSummaryJSON: outputJSON,
		UsageJSON:         usageJSON,
		ErrorJSON:         errorJSON,
		FinishedAt:        finishedAt,
		DurationMs:        durationMs,
	}); err != nil {
		if errors.Is(err, ErrLeaseNotHeld) {
			log.Printf("[gateway] late success for durable run %s rejected (lease taken over); replaying current state", runID)
			return g.lateResultReplay(ctx, payload.TenantID, runID)
		}
		return nil, fmt.Errorf("complete durable V1 run: %w", err)
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

// recordFailure 原子记录 Durable Run、Step/Event 和 V1 摘要失败状态。
// 迟到失败被 ErrLeaseNotHeld 拒绝时直接透传且不写失败审计(接管者仍在执行)。
func (g *Gateway) recordFailure(ctx context.Context, payload *AgentRunPayload, runID string, attempt int, owner, errMsg string, startedAt time.Time) error {
	finishedAt := time.Now()
	durationMs := int(finishedAt.Sub(startedAt).Milliseconds())
	errData, _ := json.Marshal(map[string]string{"code": "GATEWAY_ERROR", "message": errMsg})
	errJSON := string(errData)
	persistErr := g.durable.CompleteV1Run(ctx, &V1DurableRunCompletion{
		TenantID:   payload.TenantID,
		RunID:      runID,
		Attempt:    attempt,
		Status:     RunStatusFailed,
		LeaseOwner: owner,
		ErrorJSON:  &errJSON,
		FinishedAt: finishedAt,
		DurationMs: durationMs,
	})
	if errors.Is(persistErr, ErrLeaseNotHeld) {
		return persistErr
	}

	// 写入审计日志
	if g.auditRepo == nil {
		return persistErr
	}
	var actorUserID *string
	if payload.UserID != "" {
		actorUserID = &payload.UserID
	}
	detail := errJSON
	g.auditRepo.InsertLog(ctx, audit.AuditLogEntry{
		TraceID:         payload.TraceID,
		TenantID:        payload.TenantID,
		ActorUserID:     actorUserID,
		BusinessAppCode: &payload.BusinessAppCode,
		Action:          "agent_run_failed",
		ResourceType:    "agent_run_log",
		ResourceID:      runID,
		Status:          "failed",
		DetailJSON:      &detail,
	})
	return persistErr
}

func replayDurableRun(run *DurableRun) *AgentRunResponse {
	resp := &AgentRunResponse{
		RunID:    run.ID,
		GraphKey: run.GraphKey,
		Status:   run.Status,
		Output:   map[string]any{},
		Replayed: true,
	}
	if run.OutputSummaryJSON != nil {
		_ = json.Unmarshal([]byte(*run.OutputSummaryJSON), &resp.Output)
	}
	if run.ErrorJSON != nil {
		resp.Error = &AgentRunError{}
		_ = json.Unmarshal([]byte(*run.ErrorJSON), resp.Error)
	}
	return resp
}

func summarizeAgentInput(input map[string]any) string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	data, _ := json.Marshal(map[string]any{
		"keys":      keys,
		"key_count": len(keys),
	})
	return string(data)
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
		TenantID:        payload.TenantID,
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
	TraceID                 string         // 跨服务追踪 ID
	BusinessAppCode         string         // 业务域
	WorkflowTemplateKey     string         // 模板 key
	WorkflowTemplateVersion string         // 模板不可变版本
	GraphKey                string         // Python Graph key
	WorkflowInstanceID      string         // 工作流实例 ID
	NodeInstanceID          string         // 节点实例 ID
	ThreadTitle             string         // Durable Thread 展示标题
	Attempt                 int            // Workflow Node 执行 attempt（从 1 开始）
	Input                   map[string]any // 节点输入
	UserID                  string         // 触发用户
	TenantID                string         // authenticated workflow tenant
}
