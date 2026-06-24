package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/enterprise-agent-platform/go-platform/internal/agent"
)

// ── Asynq 任务客户端（将节点执行任务放入 Redis 队列） ──

// Worker 封装 Asynq 客户端和服务端。
// 客户端（Client）负责入队任务，服务端（Server）负责消费任务。
//
// Phase 3 新增：
//   - agentGateway: agent_graph 节点通过 Gateway 调用 Python Agent Service
//   - agentRepo: human_review 节点创建 approval_task 记录
type Worker struct {
	client       *asynq.Client
	svc          *Service          // 回调 service 的方法
	agentGateway *agent.Gateway    // Agent 调用网关（Phase 3）
	agentRepo    *agent.Repository // Agent 仓库（创建审批任务等）
}

// NewWorker 创建 Worker 实例。
// redisAddr: Redis 地址，格式 "host:port"
func NewWorker(redisAddr string, svc *Service) *Worker {
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	return &Worker{
		client: client,
		svc:    svc,
	}
}

// SetGateway 注入 Agent Gateway（Phase 3）。
// 在 main.go 中创建 Gateway 后调用，避免循环依赖。
func (w *Worker) SetGateway(gw *agent.Gateway, repo *agent.Repository) {
	w.agentGateway = gw
	w.agentRepo = repo
}

// EnqueueExecuteNode 将节点执行任务放入 Asynq 队列。
// 由 Service 层在需要执行节点时调用。
func (w *Worker) EnqueueExecuteNode(payload *ExecuteNodePayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	task := asynq.NewTask(TaskTypeExecuteNode, data)
	// 使用 workflow instance ID 作为队列名的一部分，
	// 保证同一工作流的节点按顺序执行
	_, err = w.client.Enqueue(task,
		asynq.Queue(WorkflowQueueName),
		asynq.MaxRetry(0), // 重试逻辑由 workflow 引擎控制，不由 Asynq
	)
	if err != nil {
		return fmt.Errorf("enqueue task: %w", err)
	}
	log.Printf("[worker] enqueued node %s (type=%s) for workflow %s",
		payload.NodeInstanceID, payload.NodeType, payload.WorkflowInstanceID)
	return nil
}

// Close 关闭 Asynq 客户端连接。
func (w *Worker) Close() {
	w.client.Close()
}

// ── Asynq 服务端（消费任务） ──

// StartServer 启动 Asynq worker 服务端，开始消费节点执行任务。
// 在 goroutine 中运行，持续处理队列中的任务。
func (w *Worker) StartServer(ctx context.Context, redisAddr string) error {
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				WorkflowQueueName: 10,
				"default":         1,
			},
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskTypeExecuteNode, w.handleExecuteNode)

	log.Printf("[worker] Asynq server starting, listening on Redis %s", redisAddr)
	return srv.Run(mux)
}

// handleExecuteNode 处理单个节点执行任务。
//
// 根据节点类型分发：
//   - file_upload → 文件应在上传时已完成，直接标记成功
//   - agent_graph → 通过 Gateway 调用 Python Agent Service（Phase 3）
//   - human_review → 创建审批任务，标记节点为 waiting_review
//   - system → 执行系统动作（归档等），直接标记成功
func (w *Worker) handleExecuteNode(ctx context.Context, t *asynq.Task) error {
	var payload ExecuteNodePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	log.Printf("[worker] processing node %s (type=%s) in workflow %s",
		payload.NodeInstanceID, payload.NodeType, payload.WorkflowInstanceID)

	// 1. 标记节点开始执行
	now := time.Now()
	if err := w.svc.repo.UpdateNodeStatus(ctx, payload.NodeInstanceID, NodeStatusRunning, &now, nil); err != nil {
		return fmt.Errorf("update node to running: %w", err)
	}

	// 2. 根据节点类型分发处理
	switch payload.NodeType {
	case NodeTypeFileUpload:
		return w.handleFileUpload(ctx, &payload)

	case NodeTypeAgentGraph:
		return w.handleAgentGraph(ctx, &payload)

	case NodeTypeHumanReview:
		return w.handleHumanReview(ctx, &payload)

	case NodeTypeSystem:
		return w.handleSystem(ctx, &payload)

	default:
		return fmt.Errorf("unknown node type: %s", payload.NodeType)
	}
}

// handleFileUpload 处理文件上传节点。
// 文件在创建任务时已通过 /api/v1/files 上传，此处直接标记成功并推进。
func (w *Worker) handleFileUpload(ctx context.Context, payload *ExecuteNodePayload) error {
	now := time.Now()
	if err := w.svc.repo.UpdateNodeStatus(ctx, payload.NodeInstanceID, NodeStatusSucceeded, nil, &now); err != nil {
		return fmt.Errorf("complete file_upload node: %w", err)
	}
	if err := w.svc.OnNodeCompleted(ctx, payload.NodeInstanceID, EdgeWhenSucceeded); err != nil {
		log.Printf("[worker] on node completed error: %v", err)
	}
	return nil
}

// handleAgentGraph 通过 Agent Gateway 调用 Python Agent Service。
//
// Phase 3 核心实现：
//  1. 组装 AgentRunPayload（从 workflow context + 模板信息）
//  2. 调用 Gateway.Execute（验证 → 调 Python → 记日志）
//  3. 根据返回结果更新节点状态
func (w *Worker) handleAgentGraph(ctx context.Context, payload *ExecuteNodePayload) error {
	if w.agentGateway == nil {
		// Gateway 未注入时的 fallback（不应发生）
		log.Printf("[worker] WARNING: agent gateway not set, skipping agent_graph node %s", payload.NodeInstanceID)
		now := time.Now()
		_ = w.svc.repo.UpdateNodeStatus(ctx, payload.NodeInstanceID, NodeStatusSucceeded, nil, &now)
		_ = w.svc.OnNodeCompleted(ctx, payload.NodeInstanceID, EdgeWhenSucceeded)
		return nil
	}

	// 查工作流实例获取业务上下文
	inst, err := w.svc.repo.FindInstanceByID(ctx, payload.WorkflowInstanceID)
	if err != nil {
		return fmt.Errorf("find instance: %w", err)
	}

	// 通过 Gateway 调用 Python
	gatewayPayload := &agent.AgentRunPayload{
		TraceID:             payload.TraceID,
		BusinessAppCode:     inst.BusinessAppCode,
		WorkflowTemplateKey: inst.WorkflowTemplateKey,
		GraphKey:            payload.GraphKey,
		WorkflowInstanceID:  payload.WorkflowInstanceID,
		NodeInstanceID:      payload.NodeInstanceID,
		Input:               map[string]any{"workflow_input": inst.InputJSON},
		UserID:              inst.CreatedBy,
	}

	agentResp, err := w.agentGateway.Execute(ctx, gatewayPayload)
	if err != nil {
		log.Printf("[worker] agent graph execution failed: %v", err)
		// 记录节点失败，触发重试逻辑
		if onErr := w.svc.OnNodeFailed(ctx, payload.NodeInstanceID, err.Error()); onErr != nil {
			log.Printf("[worker] on node failed error: %v", onErr)
		}
		return err
	}

	// Gateway 返回成功
	now := time.Now()
	if agentResp.Status == "succeeded" {
		// Persist agent output to node_instance for debugging and frontend display
		if agentResp.Output != nil {
			outputBytes, _ := json.Marshal(agentResp.Output)
			if err := w.svc.repo.UpdateNodeOutput(ctx, payload.NodeInstanceID, string(outputBytes)); err != nil {
				log.Printf("[worker] failed to save node output: %v", err)
			}
		}
		if err := w.svc.repo.UpdateNodeStatus(ctx, payload.NodeInstanceID, NodeStatusSucceeded, nil, &now); err != nil {
			return fmt.Errorf("complete agent_graph node: %w", err)
		}
		if err := w.svc.OnNodeCompleted(ctx, payload.NodeInstanceID, EdgeWhenSucceeded); err != nil {
			log.Printf("[worker] on node completed error: %v", err)
		}
	} else {
		// Persist error details to node_instance
		if agentResp.Error != nil {
			errBytes, _ := json.Marshal(agentResp.Error)
			if err := w.svc.repo.UpdateNodeError(ctx, payload.NodeInstanceID, string(errBytes)); err != nil {
				log.Printf("[worker] failed to save node error: %v", err)
			}
		}
		if onErr := w.svc.OnNodeFailed(ctx, payload.NodeInstanceID, "agent returned status: "+agentResp.Status); onErr != nil {
			log.Printf("[worker] on node failed error: %v", onErr)
		}
	}

	return nil
}

// handleHumanReview 处理人工审批节点。
//
// 流程：
//  1. 创建 approval_task 记录
//  2. 标记节点为 waiting_review
//  3. 更新工作流实例状态为 waiting_review
func (w *Worker) handleHumanReview(ctx context.Context, payload *ExecuteNodePayload) error {
	inst, err := w.svc.repo.FindInstanceByID(ctx, payload.WorkflowInstanceID)
	if err != nil {
		return fmt.Errorf("find instance: %w", err)
	}

	// 创建审批任务
	if w.agentRepo != nil {
		task := &agent.ApprovalTask{
			WorkflowInstanceID: payload.WorkflowInstanceID,
			NodeInstanceID:     payload.NodeInstanceID,
			BusinessAppCode:    inst.BusinessAppCode,
			Title:              fmt.Sprintf("Review: %s", inst.Title),
			Status:             "pending",
		}
		// 从模板查节点定义获取审批角色
		tmpl, tmplErr := w.svc.repo.FindTemplateByBusinessAndKey(ctx, inst.BusinessAppCode, inst.WorkflowTemplateKey)
		if tmplErr == nil {
			engine := NewEngine()
			def, defErr := engine.ParseDefinition(tmpl.DefinitionJSON)
			if defErr == nil {
				for _, n := range def.Nodes {
					if n.ID == payload.NodeKey && n.Role != "" {
						task.AssigneeRole = &n.Role
						break
					}
				}
			}
		}

		if createErr := w.agentRepo.CreateApprovalTask(ctx, task); createErr != nil {
			log.Printf("[worker] failed to create approval task: %v", createErr)
		} else {
			log.Printf("[worker] approval task %s created for node %s", task.ID, payload.NodeInstanceID)
		}
	}

	// 标记节点为 waiting_review
	now := time.Now()
	if err := w.svc.repo.UpdateNodeStatus(ctx, payload.NodeInstanceID, NodeStatusWaitingReview, nil, &now); err != nil {
		return fmt.Errorf("set human_review node to waiting: %w", err)
	}
	// 更新工作流实例状态
	_ = w.svc.repo.UpdateInstanceStatus(ctx, payload.WorkflowInstanceID, StatusWaitingReview, nil, nil)
	log.Printf("[worker] human_review node %s: waiting for approval", payload.NodeInstanceID)

	return nil
}

// handleSystem 处理系统节点（归档等）。
// 直接标记成功并推进流程。
func (w *Worker) handleSystem(ctx context.Context, payload *ExecuteNodePayload) error {
	now := time.Now()
	if err := w.svc.repo.UpdateNodeStatus(ctx, payload.NodeInstanceID, NodeStatusSucceeded, nil, &now); err != nil {
		return fmt.Errorf("complete system node: %w", err)
	}
	if payload.NodeKey == "archive" {
		inst, err := w.svc.repo.FindInstanceByID(ctx, payload.WorkflowInstanceID)
		if err != nil {
			return fmt.Errorf("find instance for archive: %w", err)
		}
		if w.agentRepo != nil {
			output, err := w.agentRepo.LatestRunOutput(ctx, payload.WorkflowInstanceID)
			if err != nil && err != pgx.ErrNoRows {
				return fmt.Errorf("load latest agent output: %w", err)
			}
			if output != nil {
				if err := w.svc.repo.UpdateInstanceOutput(ctx, payload.WorkflowInstanceID, *output); err != nil {
					return fmt.Errorf("archive output: %w", err)
				}
				// Also save output to the system node for per-node display
				if err := w.svc.repo.UpdateNodeOutput(ctx, payload.NodeInstanceID, *output); err != nil {
					log.Printf("[worker] failed to save archive node output: %v", err)
				}
			}
		}
		if err := w.svc.repo.UpdateInstanceStatus(ctx, payload.WorkflowInstanceID, StatusArchived, nil, &now); err != nil {
			return fmt.Errorf("archive workflow: %w", err)
		}
		w.svc.auditLog(ctx, "", inst.BusinessAppCode, inst.TraceID, "workflow_archived", payload.WorkflowInstanceID, StatusArchived, nil)
		return nil
	}
	if err := w.svc.OnNodeCompleted(ctx, payload.NodeInstanceID, EdgeWhenSucceeded); err != nil {
		log.Printf("[worker] on node completed error: %v", err)
	}
	return nil
}
