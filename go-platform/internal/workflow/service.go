package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/google/uuid"
)

// Service 实现工作流业务逻辑。
// 位于 Repository（数据层）和 Handler（HTTP 层）之间，
// 编排状态机、模板解释、持久化和异步任务。
type Service struct {
	repo      workflowRepository
	auditRepo workflowAuditLogger
	engine    *Engine
	worker    nodeEnqueuer
	runCancel runCanceller
}

type runCanceller interface {
	CancelWorkflowRuns(ctx context.Context, tenantID, workflowInstanceID, reason, requestedBy string) error
}

// SetRunCanceller 注入 Durable Run 取消控制器(工作流取消时联动取消活动 Run)。
func (s *Service) SetRunCanceller(canceller runCanceller) {
	s.runCancel = canceller
}

type workflowAuditLogger interface {
	InsertLog(ctx context.Context, entry audit.AuditLogEntry) (string, time.Time, error)
}

type workflowRepository interface {
	FindTemplatesByBusinessApp(ctx context.Context, businessAppCode string) ([]Template, error)
	ListTemplates(ctx context.Context, businessAppCode, status, graphKey string) ([]Template, error)
	FindTemplateByBusinessAndKey(ctx context.Context, businessAppCode, templateKey string) (*Template, error)
	FindInstanceByID(ctx context.Context, id string) (*Instance, error)
	FindInstanceByIDForTenant(ctx context.Context, tenantID, id string) (*Instance, error)
	FindInstanceByIdempotencyKey(ctx context.Context, tenantID, createdBy, key string) (*Instance, error)
	ListInstances(ctx context.Context, tenantID, businessAppCode, status, createdBy string, page, pageSize int) ([]Instance, int, error)
	FindNodeInstancesByWorkflow(ctx context.Context, workflowInstanceID string) ([]NodeInstance, error)
	FindNodeInstanceByID(ctx context.Context, id string) (*NodeInstance, error)
	CreateInstance(ctx context.Context, inst *Instance) error
	CreateNodeInstances(ctx context.Context, workflowInstanceID string, nodes []TemplateNode) error
	UpdateInstanceStatus(ctx context.Context, id, status string, startedAt, finishedAt *time.Time) error
	UpdateInstanceOutput(ctx context.Context, id string, outputJSON string) error
	UpdateNodeStatus(ctx context.Context, id, status string, startedAt, finishedAt *time.Time) error
	UpdateNodeInput(ctx context.Context, id string, inputJSON string) error
	UpdateNodeOutput(ctx context.Context, id string, outputJSON string) error
	UpdateNodeError(ctx context.Context, id string, errorJSON string) error
	FindPendingNodeByWorkflow(ctx context.Context, workflowInstanceID string) (*NodeInstance, error)
	CancelPendingNodes(ctx context.Context, workflowInstanceID string) error
}

type nodeEnqueuer interface {
	EnqueueExecuteNode(payload *ExecuteNodePayload) error
}

// NewService 创建 Service 实例。
func NewService(repo *Repository, auditRepo *audit.Repository, engine *Engine, worker *Worker) *Service {
	s := &Service{
		repo:   repo,
		engine: engine,
		worker: worker,
	}
	if auditRepo != nil {
		s.auditRepo = auditRepo
	}
	return s
}

// SetWorker 注入 Worker 实例。
// Worker 和 Service 互相引用，通过先创建 Service → 创建 Worker → SetWorker 打破循环。
func (s *Service) SetWorker(w *Worker) {
	s.worker = w
}

// ── 查询 ──

// GetTemplates 查询某业务下所有可用模板。
func (s *Service) GetTemplates(ctx context.Context, businessAppCode string) ([]Template, error) {
	return s.repo.FindTemplatesByBusinessApp(ctx, businessAppCode)
}

func (s *Service) ListTemplates(ctx context.Context, businessAppCode, status, graphKey string) ([]Template, error) {
	return s.repo.ListTemplates(ctx, businessAppCode, status, graphKey)
}

// GetInstance 查询单个工作流实例。
func (s *Service) GetInstance(ctx context.Context, tenantID, id string) (*Instance, error) {
	return s.repo.FindInstanceByIDForTenant(ctx, tenantID, id)
}

// ListInstances 分页查询实例列表。
func (s *Service) ListInstances(ctx context.Context, tenantID, businessAppCode, status, createdBy string, page, pageSize int) ([]Instance, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	return s.repo.ListInstances(ctx, tenantID, businessAppCode, status, createdBy, page, pageSize)
}

// GetNodeInstances 查询工作流的所有节点实例。

func (s *Service) GetNodeInstances(ctx context.Context, tenantID, workflowInstanceID string) ([]NodeInstance, error) {
	if _, err := s.repo.FindInstanceByIDForTenant(ctx, tenantID, workflowInstanceID); err != nil {
		return nil, err
	}
	return s.repo.FindNodeInstancesByWorkflow(ctx, workflowInstanceID)
}

// ── 创建实例 ──

// CreateInstance 根据模板创建工作流实例。
//
// 流程：
//  1. 查询模板（by business_app_code + template_key）
//  2. 解析模板定义
//  3. 创建 workflow_instance 记录（status = draft）
//  4. 从模板节点列表创建所有 node_instance 记录（status = pending）
//  5. 返回创建的实例信息
func (s *Service) CreateInstance(ctx context.Context, userID, tenantID, idempotencyKey string, req CreateInstanceRequest) (*CreateInstanceResponse, error) {
	if idempotencyKey != "" {
		if existing, err := s.repo.FindInstanceByIdempotencyKey(ctx, tenantID, userID, idempotencyKey); err == nil {
			return createInstanceResponse(existing), nil
		}
	}
	// 1. 查模板
	tmpl, err := s.repo.FindTemplateByBusinessAndKey(ctx, req.BusinessAppCode, req.WorkflowTemplateKey)
	if err != nil {
		return nil, fmt.Errorf("find template: %w", err)
	}

	// 2. 解析模板定义
	def, err := s.engine.ParseDefinition(tmpl.DefinitionJSON)
	if err != nil {
		return nil, err
	}

	// 3. 序列化输入
	inputJSON, _ := json.Marshal(req.Input)
	traceID := uuid.New().String()

	// 4. 创建实例
	inst := &Instance{
		BusinessAppCode:         tmpl.BusinessAppCode,
		WorkflowTemplateID:      tmpl.ID,
		WorkflowTemplateKey:     tmpl.WorkflowTemplateKey,
		WorkflowTemplateVersion: tmpl.Version,
		GraphKey:                tmpl.GraphKey,
		Title:                   req.Title,
		Status:                  StatusDraft,
		InputJSON:               string(inputJSON),
		CreatedBy:               userID,
		TraceID:                 traceID,
		TenantID:                tenantID,
	}
	if idempotencyKey != "" {
		inst.IdempotencyKey = &idempotencyKey
	}
	if err := s.repo.CreateInstance(ctx, inst); err != nil {
		if idempotencyKey != "" {
			if existing, findErr := s.repo.FindInstanceByIdempotencyKey(ctx, tenantID, userID, idempotencyKey); findErr == nil {
				return createInstanceResponse(existing), nil
			}
		}
		return nil, fmt.Errorf("create instance: %w", err)
	}

	// 5. 创建所有节点实例
	if err := s.repo.CreateNodeInstances(ctx, inst.ID, def.Nodes); err != nil {
		return nil, fmt.Errorf("create nodes: %w", err)
	}

	s.auditLog(ctx, userID, inst.BusinessAppCode, inst.TraceID, "workflow_instance_created", inst.ID, StatusDraft, nil)

	return createInstanceResponse(inst), nil
}

func createInstanceResponse(inst *Instance) *CreateInstanceResponse {
	return &CreateInstanceResponse{
		ID:                      inst.ID,
		BusinessAppCode:         inst.BusinessAppCode,
		WorkflowTemplateKey:     inst.WorkflowTemplateKey,
		WorkflowTemplateVersion: inst.WorkflowTemplateVersion,
		GraphKey:                inst.GraphKey,
		Title:                   inst.Title,
		Status:                  inst.Status,
		TraceID:                 inst.TraceID,
	}
}

// ── 启动工作流 ──

// StartWorkflow 启动 draft 状态的工作流。
//
// 流程：
//  1. 校验实例状态（必须是 draft）
//  2. 更新实例状态为 running，记录开始时间
//  3. 找到入口节点（没有入边的节点）
//  4. 将入口节点入队（Asynq 异步执行）
func (s *Service) StartWorkflow(ctx context.Context, userID, tenantID, instanceID string) (*StartResponse, error) {
	// 1. 查实例
	inst, err := s.repo.FindInstanceByIDForTenant(ctx, tenantID, instanceID)
	if err != nil {
		return nil, fmt.Errorf("find instance: %w", err)
	}

	// 2. 校验是否可以启动
	if err := s.engine.CanStartInstance(inst); err != nil {
		return nil, err
	}

	// 3. 更新实例状态
	now := time.Now()
	if err := s.repo.UpdateInstanceStatus(ctx, instanceID, StatusRunning, &now, nil); err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	// 4. 查模板定义 → 找入口节点
	tmpl, err := s.repo.FindTemplateByBusinessAndKey(ctx, inst.BusinessAppCode, inst.WorkflowTemplateKey)
	if err != nil {
		return nil, fmt.Errorf("find template: %w", err)
	}
	def, err := s.engine.ParseDefinition(tmpl.DefinitionJSON)
	if err != nil {
		return nil, err
	}

	entryNodes := s.engine.GetEntryNodes(def)
	if len(entryNodes) == 0 {
		return nil, fmt.Errorf("no entry nodes found in template")
	}

	// 5. 查入口节点对应的 node_instance
	nodes, err := s.repo.FindNodeInstancesByWorkflow(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("find nodes: %w", err)
	}

	s.auditLog(ctx, userID, inst.BusinessAppCode, inst.TraceID, "workflow_instance_started", instanceID, StatusRunning, nil)

	// 找到入口节点并逐个入队
	entryKeys := make(map[string]bool)
	for _, n := range entryNodes {
		entryKeys[n.ID] = true
	}
	for _, n := range nodes {
		if entryKeys[n.NodeKey] {
			if s.worker != nil {
				if err := s.worker.EnqueueExecuteNode(&ExecuteNodePayload{
					WorkflowInstanceID: instanceID,
					NodeInstanceID:     n.ID,
					Attempt:            n.RetryCount + 1,
					NodeType:           n.NodeType,
					NodeKey:            n.NodeKey,
					GraphKey:           inst.GraphKey,
					TraceID:            inst.TraceID,
				}); err != nil {
					finishedAt := time.Now()
					_ = s.repo.UpdateInstanceStatus(ctx, instanceID, StatusFailed, nil, &finishedAt)
					s.auditLog(ctx, userID, inst.BusinessAppCode, inst.TraceID, "workflow_instance_failed", instanceID, StatusFailed, nil)
					return nil, fmt.Errorf("enqueue entry node %s: %w", n.NodeKey, err)
				}
			}
		}
	}

	return &StartResponse{ID: instanceID, Status: StatusRunning}, nil
}

// ── 取消工作流 ──

// CancelWorkflow 取消工作流实例。
//
// 流程：
//  1. 查实例并校验状态
//  2. 将所有 pending/running/waiting_review 的节点设为 cancelled
//  3. 更新实例状态为 cancelled
func (s *Service) CancelWorkflow(ctx context.Context, userID, tenantID, instanceID string) (*StartResponse, error) {
	inst, err := s.repo.FindInstanceByIDForTenant(ctx, tenantID, instanceID)
	if err != nil {
		return nil, fmt.Errorf("find instance: %w", err)
	}

	if err := s.engine.CanCancelInstance(inst); err != nil {
		return nil, err
	}

	// 取消所有活跃节点
	if err := s.repo.CancelPendingNodes(ctx, instanceID); err != nil {
		return nil, fmt.Errorf("cancel nodes: %w", err)
	}

	// 联动取消该工作流下的活动 Durable Run(best-effort)。
	if s.runCancel != nil {
		if err := s.runCancel.CancelWorkflowRuns(ctx, tenantID, instanceID, "workflow cancelled", userID); err != nil {
			log.Printf("[workflow] cancel workflow runs %s failed: %v", instanceID, err)
		}
	}

	// 更新实例状态
	now := time.Now()
	if err := s.repo.UpdateInstanceStatus(ctx, instanceID, StatusCancelled, nil, &now); err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	s.auditLog(ctx, userID, inst.BusinessAppCode, inst.TraceID, "workflow_instance_cancelled", instanceID, StatusCancelled, nil)

	return &StartResponse{ID: instanceID, Status: StatusCancelled}, nil
}

// ── 重试节点 ──

// RetryNode 重试失败的节点。
//
// 流程：
//  1. 查节点并校验可否重试
//  2. 更新节点状态为 running
//  3. 如果实例状态是 failed，恢复为 running
//  4. 将节点重新入队
func (s *Service) RetryNode(ctx context.Context, userID, tenantID, instanceID, nodeInstanceID string) (*StartResponse, error) {
	if _, err := s.repo.FindInstanceByIDForTenant(ctx, tenantID, instanceID); err != nil {
		return nil, fmt.Errorf("find workflow instance: %w", err)
	}
	node, err := s.repo.FindNodeInstanceByID(ctx, nodeInstanceID)
	if err != nil {
		return nil, fmt.Errorf("find node: %w", err)
	}
	if node.WorkflowInstanceID != instanceID {
		return nil, fmt.Errorf("node does not belong to workflow instance")
	}

	// 校验是否可以重试
	if err := s.engine.CanRetryNode(node); err != nil {
		return nil, err
	}

	// 更新节点状态
	now := time.Now()
	if err := s.repo.UpdateNodeStatus(ctx, nodeInstanceID, NodeStatusRunning, &now, nil); err != nil {
		return nil, fmt.Errorf("update node status: %w", err)
	}

	// 如果实例是 failed，恢复到 running
	inst, err := s.repo.FindInstanceByID(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("find instance: %w", err)
	}
	if inst.Status == StatusFailed {
		if err := s.repo.UpdateInstanceStatus(ctx, instanceID, StatusRunning, nil, nil); err != nil {
			return nil, fmt.Errorf("update instance status: %w", err)
		}
	}

	// 重新入队
	if s.worker != nil {
		if err := s.worker.EnqueueExecuteNode(&ExecuteNodePayload{
			WorkflowInstanceID: instanceID,
			NodeInstanceID:     nodeInstanceID,
			Attempt:            node.RetryCount + 1,
			NodeType:           node.NodeType,
			NodeKey:            node.NodeKey,
			GraphKey:           inst.GraphKey,
			TraceID:            inst.TraceID,
		}); err != nil {
			if onErr := s.OnNodeFailed(ctx, nodeInstanceID, fmt.Sprintf("enqueue retry node: %v", err)); onErr != nil {
				return nil, fmt.Errorf("enqueue retry node: %w; additionally failed to mark node failed: %v", err, onErr)
			}
			return nil, fmt.Errorf("enqueue retry node: %w", err)
		}
	}

	detailBytes, _ := json.Marshal(map[string]string{"node_instance_id": nodeInstanceID, "node_key": node.NodeKey})
	detail := string(detailBytes)
	s.auditLog(ctx, userID, inst.BusinessAppCode, inst.TraceID, "workflow_node_retried", instanceID, StatusRunning, &detail)

	return &StartResponse{ID: instanceID, Status: StatusRunning}, nil
}

// ── 节点完成回调（由 worker 调用） ──

// OnNodeCompleted worker 完成一个节点后调用此方法。
//
// 流程：
//  1. 查模板定义和当前节点
//  2. 根据边条件找下一个节点
//  3. 有下一节点 → 入队执行
//  4. 无下一节点 → 判断是否所有节点 succeeded → 更新实例为 approved
func (s *Service) OnNodeCompleted(ctx context.Context, nodeInstanceID, edgeWhen string) error {
	node, err := s.repo.FindNodeInstanceByID(ctx, nodeInstanceID)
	if err != nil {
		return fmt.Errorf("find node: %w", err)
	}

	inst, err := s.repo.FindInstanceByID(ctx, node.WorkflowInstanceID)
	if err != nil {
		return fmt.Errorf("find instance: %w", err)
	}

	// 查模板定义
	tmpl, err := s.repo.FindTemplateByBusinessAndKey(ctx, inst.BusinessAppCode, inst.WorkflowTemplateKey)
	if err != nil {
		return fmt.Errorf("find template: %w", err)
	}
	def, err := s.engine.ParseDefinition(tmpl.DefinitionJSON)
	if err != nil {
		return err
	}

	// 找下一个节点
	nextNodes := s.engine.GetNextNodes(def, node.NodeKey, edgeWhen)

	if len(nextNodes) == 0 {
		// 没有下一节点 → 流程可能结束了
		// 检查所有节点是否都已完成
		allNodes, err := s.repo.FindNodeInstancesByWorkflow(ctx, inst.ID)
		if err != nil {
			return fmt.Errorf("find nodes for completion check: %w", err)
		}
		allDone := true
		for _, n := range allNodes {
			if n.Status != NodeStatusSucceeded && n.Status != NodeStatusSkipped && n.Status != NodeStatusCancelled {
				allDone = false
				break
			}
		}
		if allDone {
			now := time.Now()
			_ = s.repo.UpdateInstanceStatus(ctx, inst.ID, StatusApproved, nil, &now)
			s.auditLog(ctx, inst.CreatedBy, inst.BusinessAppCode, inst.TraceID, "workflow_instance_approved", inst.ID, StatusApproved, nil)
		}
		return nil
	}

	// 将下一批节点入队
	allNodes, err := s.repo.FindNodeInstancesByWorkflow(ctx, inst.ID)
	if err != nil {
		return fmt.Errorf("find nodes for next dispatch: %w", err)
	}
	nodeMap := make(map[string]NodeInstance)
	for _, n := range allNodes {
		nodeMap[n.NodeKey] = n
	}

	for _, n := range nextNodes {
		if ni, ok := nodeMap[n.ID]; ok {
			if s.worker != nil {
				if err := s.worker.EnqueueExecuteNode(&ExecuteNodePayload{
					WorkflowInstanceID: inst.ID,
					NodeInstanceID:     ni.ID,
					Attempt:            ni.RetryCount + 1,
					NodeType:           ni.NodeType,
					NodeKey:            ni.NodeKey,
					GraphKey:           inst.GraphKey,
					TraceID:            inst.TraceID,
				}); err != nil {
					if onErr := s.OnNodeFailed(ctx, ni.ID, fmt.Sprintf("enqueue next node: %v", err)); onErr != nil {
						return fmt.Errorf("enqueue next node %s: %w; additionally failed to mark node failed: %v", ni.NodeKey, err, onErr)
					}
					return fmt.Errorf("enqueue next node %s: %w", ni.NodeKey, err)
				}
			}
		} else {
			return fmt.Errorf("node instance for template node %s not found", n.ID)
		}
	}

	return nil
}

// OnNodeFailed worker 节点执行失败时调用此方法。
func (s *Service) CompleteHumanReviewNode(ctx context.Context, nodeInstanceID, decision, userID, comment string) error {
	node, err := s.repo.FindNodeInstanceByID(ctx, nodeInstanceID)
	if err != nil {
		return fmt.Errorf("find review node: %w", err)
	}
	if node.NodeType != NodeTypeHumanReview {
		return fmt.Errorf("node %s is not a human_review node", nodeInstanceID)
	}
	inst, err := s.repo.FindInstanceByID(ctx, node.WorkflowInstanceID)
	if err != nil {
		return fmt.Errorf("find instance: %w", err)
	}

	now := time.Now()
	detailBytes, _ := json.Marshal(map[string]string{"comment": comment, "node_id": nodeInstanceID})
	detail := string(detailBytes)

	switch decision {
	case "approved":
		if err := s.repo.UpdateNodeStatus(ctx, nodeInstanceID, NodeStatusSucceeded, nil, &now); err != nil {
			return fmt.Errorf("mark review node succeeded: %w", err)
		}
		if err := s.repo.UpdateInstanceStatus(ctx, inst.ID, StatusApproved, nil, nil); err != nil {
			return fmt.Errorf("mark workflow approved: %w", err)
		}
		s.auditLog(ctx, userID, inst.BusinessAppCode, inst.TraceID, "workflow_human_review_approved", inst.ID, StatusApproved, &detail)
		return s.OnNodeCompleted(ctx, nodeInstanceID, EdgeWhenApproved)

	case "rejected":
		if err := s.repo.UpdateNodeStatus(ctx, nodeInstanceID, NodeStatusFailed, nil, &now); err != nil {
			return fmt.Errorf("mark review node failed: %w", err)
		}
		if err := s.repo.UpdateInstanceStatus(ctx, inst.ID, StatusRejected, nil, &now); err != nil {
			return fmt.Errorf("mark workflow rejected: %w", err)
		}
		s.auditLog(ctx, userID, inst.BusinessAppCode, inst.TraceID, "workflow_human_review_rejected", inst.ID, StatusRejected, &detail)
		return nil

	default:
		return fmt.Errorf("unknown review decision: %s", decision)
	}
}

func (s *Service) ContinueAfterHumanReviewNode(ctx context.Context, nodeInstanceID, decision, userID, comment string) error {
	node, err := s.repo.FindNodeInstanceByID(ctx, nodeInstanceID)
	if err != nil {
		return fmt.Errorf("find review node: %w", err)
	}
	if node.NodeType != NodeTypeHumanReview {
		return fmt.Errorf("node %s is not a human_review node", nodeInstanceID)
	}
	inst, err := s.repo.FindInstanceByID(ctx, node.WorkflowInstanceID)
	if err != nil {
		return fmt.Errorf("find instance: %w", err)
	}

	detailBytes, _ := json.Marshal(map[string]string{"comment": comment, "node_id": nodeInstanceID})
	detail := string(detailBytes)

	switch decision {
	case "approved":
		s.auditLog(ctx, userID, inst.BusinessAppCode, inst.TraceID, "workflow_human_review_approved", inst.ID, StatusApproved, &detail)
		return s.OnNodeCompleted(ctx, nodeInstanceID, EdgeWhenApproved)
	case "rejected":
		s.auditLog(ctx, userID, inst.BusinessAppCode, inst.TraceID, "workflow_human_review_rejected", inst.ID, StatusRejected, &detail)
		return nil
	default:
		return fmt.Errorf("unknown review decision: %s", decision)
	}
}

func (s *Service) OnNodeFailed(ctx context.Context, nodeInstanceID, errorMsg string) error {
	node, err := s.repo.FindNodeInstanceByID(ctx, nodeInstanceID)
	if err != nil {
		return fmt.Errorf("find node: %w", err)
	}

	// 更新节点错误信息（用 json.Marshal 防止 errorMsg 中的引号破坏 JSON）
	errData, _ := json.Marshal(map[string]string{"message": errorMsg})
	if err := s.repo.UpdateNodeError(ctx, nodeInstanceID, string(errData)); err != nil {
		return fmt.Errorf("update node error: %w", err)
	}

	// 标记节点失败
	now := time.Now()
	if err := s.repo.UpdateNodeStatus(ctx, nodeInstanceID, NodeStatusFailed, nil, &now); err != nil {
		return fmt.Errorf("update node status: %w", err)
	}

	// 如果重试次数已用完，整个实例标记为 failed
	if node.RetryCount+1 >= node.MaxRetries {
		now := time.Now()
		_ = s.repo.UpdateInstanceStatus(ctx, node.WorkflowInstanceID, StatusFailed, nil, &now)
		// 查找实例信息用于审计日志
		if inst, err := s.repo.FindInstanceByID(ctx, node.WorkflowInstanceID); err == nil {
			s.auditLog(ctx, inst.CreatedBy, inst.BusinessAppCode, inst.TraceID, "workflow_instance_failed", node.WorkflowInstanceID, StatusFailed, nil)
		}
	}

	return nil
}

// CompleteAgentRunNode 在 Durable Run 终态事件到达时推进 agent_graph 节点。
// 幂等:节点已不在 running(已推进/已失败/等待审批)时直接返回。
func (s *Service) CompleteAgentRunNode(ctx context.Context, nodeInstanceID, runID, status string, outputJSON, errorJSON *string) error {
	node, err := s.repo.FindNodeInstanceByID(ctx, nodeInstanceID)
	if err != nil {
		return fmt.Errorf("find agent node: %w", err)
	}
	// 幂等:节点已推进/已失败时不再处理;waiting_review 需继续处理(运行时
	// 中断经审批 Resume 后,Run 终态事件要把它从 waiting_review 推进到终态)。
	if node.Status != NodeStatusRunning && node.Status != NodeStatusWaitingReview {
		return nil
	}
	inst, err := s.repo.FindInstanceByID(ctx, node.WorkflowInstanceID)
	if err != nil {
		return fmt.Errorf("find instance: %w", err)
	}

	now := time.Now()
	switch status {
	case "succeeded":
		if outputJSON != nil {
			if err := s.repo.UpdateNodeOutput(ctx, nodeInstanceID, *outputJSON); err != nil {
				log.Printf("[workflow] save agent run output for node %s: %v", nodeInstanceID, err)
			}
		}
		if err := s.repo.UpdateNodeStatus(ctx, nodeInstanceID, NodeStatusSucceeded, nil, &now); err != nil {
			return fmt.Errorf("mark agent node succeeded: %w", err)
		}
		s.auditLog(ctx, inst.CreatedBy, inst.BusinessAppCode, inst.TraceID, "agent_run_succeeded", nodeInstanceID, "succeeded", outputJSON)
		return s.OnNodeCompleted(ctx, nodeInstanceID, EdgeWhenSucceeded)

	case "failed", "cancelled":
		message := "agent run failed"
		action := "agent_run_failed"
		if status == "cancelled" {
			message = "agent run cancelled"
			action = "agent_run_cancelled"
		}
		if errorJSON != nil && *errorJSON != "" {
			message = *errorJSON
		}
		s.auditLog(ctx, inst.CreatedBy, inst.BusinessAppCode, inst.TraceID, action, nodeInstanceID, status, errorJSON)
		return s.OnNodeFailed(ctx, nodeInstanceID, message)

	default:
		return fmt.Errorf("unexpected agent run status %q", status)
	}
}

// auditLog 写入一条审计日志（忽略错误）。
func (s *Service) auditLog(ctx context.Context, userID, businessAppCode, traceID, action, resourceID, status string, detailJSON *string) {
	if s.auditRepo == nil {
		return
	}
	if detailJSON == nil {
		emptyDetail := "{}"
		detailJSON = &emptyDetail
	}
	var actorID *string
	if userID != "" {
		actorID = &userID
	}
	tenantID := ""
	if inst, err := s.repo.FindInstanceByID(ctx, resourceID); err == nil {
		tenantID = inst.TenantID
	}
	s.auditRepo.InsertLog(ctx, audit.AuditLogEntry{
		TraceID:         traceID,
		TenantID:        tenantID,
		ActorUserID:     actorID,
		BusinessAppCode: &businessAppCode,
		Action:          action,
		ResourceType:    "workflow_instance",
		ResourceID:      resourceID,
		Status:          status,
		DetailJSON:      detailJSON,
	})
}
