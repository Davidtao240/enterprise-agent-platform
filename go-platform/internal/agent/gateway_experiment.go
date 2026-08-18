package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// StartExperimentRun M5-C: 创建实验 Run(Shadow 复制 / Replay 回放的独立执行)。
//
// 与生产调度的区别(TRACE_AND_EVAL.md §4.6):
//   - 不经过 ExperimentRouter(影子不再二次分叉);
//   - 不挂 workflow_instance_id/node_instance_id(不触发工作流节点推进);
//   - metadata_json 写入实验标记(shadow=true / replay=true);
//   - 不写 agent_run_logs 兼容行(天然排除出 Eval 生产指标)。
//
// 要求 Runtime V2 已配置;graphKey 允许与 payload.GraphKey 不同(算法对比)。
func (g *Gateway) StartExperimentRun(ctx context.Context, payload *AgentRunPayload, graphKey string, metadata map[string]any) (*RuntimeV2AcceptedResponse, error) {
	if payload == nil {
		return nil, fmt.Errorf("experiment run: payload is required")
	}
	if graphKey == "" {
		return nil, fmt.Errorf("experiment run: graph_key is required")
	}
	if g.runtimeV2 == nil || g.durableV2 == nil {
		return nil, fmt.Errorf("experiment run: Runtime V2 gateway is not configured")
	}
	graph, err := g.repo.FindGraphByKey(ctx, graphKey)
	if err != nil {
		return nil, fmt.Errorf("experiment run: graph_key %s not found: %w", graphKey, err)
	}
	if graph.Status != "active" || graph.BusinessAppCode != payload.BusinessAppCode {
		return nil, fmt.Errorf("experiment run: graph_key %s is not active for business app %s", graphKey, payload.BusinessAppCode)
	}
	if err := g.validateDomainPolicy(ctx, payload.BusinessAppCode, graphKey); err != nil {
		return nil, fmt.Errorf("experiment run: domain policy violation: %w", err)
	}

	configuration := RuntimeV2Configuration{
		AgentDefinitionVersion: graph.Version,
		ProfileOrSkillVersion:  payload.WorkflowTemplateVersion,
		ModelConfigVersion:      g.modelConfigVersion,
	}
	if configuration.ProfileOrSkillVersion == "" {
		configuration.ProfileOrSkillVersion = "experiment"
	}
	if configuration.ModelConfigVersion == "" {
		configuration.ModelConfigVersion = "model:default"
	}
	budget := RuntimeV2Budget{MaxSteps: 30}
	runID := uuid.NewString()

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("experiment run: marshal metadata: %w", err)
	}
	metadataString := string(metadataJSON)
	configurationSnapshot, err := json.Marshal(map[string]any{
		"protocol_version": "2.0",
		"graph":            RuntimeV2GraphIdentity{Key: graph.GraphKey, Version: graph.Version},
		"configuration":    configuration,
	})
	if err != nil {
		return nil, fmt.Errorf("experiment run: marshal configuration snapshot: %w", err)
	}
	budgetJSON, err := json.Marshal(budget)
	if err != nil {
		return nil, fmt.Errorf("experiment run: marshal budget: %w", err)
	}
	budgetString := string(budgetJSON)
	threadTitle := payload.ThreadTitle
	if threadTitle == "" {
		threadTitle = "experiment:" + runID
	}
	run, created, err := g.durableV2.StartV2Run(ctx, &V2DurableRunStart{
		RunID: runID, TenantID: payload.TenantID, CreatedBy: payload.UserID,
		BusinessAppCode: payload.BusinessAppCode,
		ThreadTitle:     threadTitle,
		TraceID:         payload.TraceID,
		GraphKey:        graph.GraphKey, GraphVersion: graph.Version,
		ConfigurationSnapshotJSON: string(configurationSnapshot),
		BudgetJSON:                &budgetString,
		MetadataJSON:              &metadataString,
	})
	if err != nil {
		return nil, fmt.Errorf("experiment run: create durable run: %w", err)
	}
	if !created {
		return nil, fmt.Errorf("experiment run: run %s already exists", run.ID)
	}
	request := &RuntimeV2StartRequest{
		ProtocolVersion: "2.0", RunID: run.ID, ThreadID: run.ThreadID, TraceID: payload.TraceID,
		BusinessAppCode: payload.BusinessAppCode,
		Graph:           RuntimeV2GraphIdentity{Key: run.GraphKey, Version: run.GraphVersion},
		Configuration:   configuration, Input: payload.Input,
		TrustedContext: RuntimeV2TrustedContext{UserID: payload.UserID, TenantID: payload.TenantID},
		Budget:         budget, Attempt: run.Attempt, IdempotencyKey: "start:" + run.ID,
	}
	accepted, err := g.runtimeV2.Start(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("experiment run: start runtime: %w", err)
	}
	return accepted, nil
}

// forkShadowRun best-effort 异步复制一份流量到影子版本(不阻塞、不影响主链路;
// 失败仅记录日志)。影子 Run 与主 Run 共享 trace_id,便于对比串联。
func (g *Gateway) forkShadowRun(payload *AgentRunPayload, route ExperimentRoute, primaryRunID string) {
	if g.runtimeV2 == nil || g.durableV2 == nil {
		log.Printf("[gateway] shadow fork skipped: Runtime V2 not configured (primary_run=%s)", primaryRunID)
		return
	}
	shadowPayload := *payload
	shadowPayload.GraphKey = route.ShadowGraphKey
	shadowPayload.WorkflowInstanceID = ""
	shadowPayload.NodeInstanceID = ""
	metadata := map[string]any{
		"shadow":         true,
		"rule_id":        route.ShadowRuleID,
		"primary_run_id": primaryRunID,
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		accepted, err := g.StartExperimentRun(ctx, &shadowPayload, route.ShadowGraphKey, metadata)
		if err != nil {
			log.Printf("[gateway] shadow fork failed (primary_run=%s): %v", primaryRunID, err)
			return
		}
		if g.experiments != nil {
			if err := g.experiments.RecordShadowExecution(ctx, payload.TenantID, route.ShadowRuleID, primaryRunID, accepted.RunID); err != nil {
				log.Printf("[gateway] record shadow execution failed (primary=%s shadow=%s): %v", primaryRunID, accepted.RunID, err)
			}
		}
		log.Printf("[gateway] shadow forked: primary_run=%s shadow_run=%s graph=%s", primaryRunID, accepted.RunID, route.ShadowGraphKey)
	}()
}
