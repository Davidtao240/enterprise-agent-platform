package experiment

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/enterprise-agent-platform/go-platform/internal/agent"
)

// Router 实现 agent.ExperimentRouter(Gateway 流量切分单一收口的决策侧)。
//
// 决策规则(Spec §4.6):
//   - Canary: hash(run_id+":canary") % 100 < 当前阶梯百分比 → 路由到候选版本;
//   - Shadow: hash(run_id+":shadow") % 100 < 规则流量百分比 → 异步复制到影子版本;
//   - 两路哈希种子独立,避免取样完全相关;
//   - 决策只读不写,失败不阻断主链路之外返回错误由 Gateway 拒绝执行(安全优先)。
type Router struct {
	repo routerStore
}

type routerStore interface {
	FindActiveCanaryRelease(ctx context.Context, tenantID, businessAppCode, graphKey string) (*CanaryRelease, error)
	FindActiveShadowRule(ctx context.Context, tenantID, businessAppCode, graphKey string) (*ShadowRule, error)
	InsertShadowExecution(ctx context.Context, exec *ShadowExecution) error
}

// NewRouter 创建 Router(repo 为 nil 时所有请求直投,等价于关闭实验分流)。
func NewRouter(repo routerStore) *Router { return &Router{repo: repo} }

// ResolveRoute 按 run_id 确定性取样,做出 Canary/Shadow 决策。
func (r *Router) ResolveRoute(ctx context.Context, tenantID, businessAppCode, graphKey, runID string) (agent.ExperimentRoute, error) {
	route := agent.ExperimentRoute{ResolvedGraphKey: graphKey}
	if r.repo == nil {
		return route, nil
	}
	if release, err := r.repo.FindActiveCanaryRelease(ctx, tenantID, businessAppCode, graphKey); err != nil {
		return route, fmt.Errorf("resolve canary: %w", err)
	} else if release != nil {
		if percent := release.CurrentStagePercent(); percent > 0 && sampleHit(runID, "canary", percent) {
			route.ResolvedGraphKey = release.CandidateGraphKey
			route.CanaryReleaseID = release.ID
		}
	}
	if rule, err := r.repo.FindActiveShadowRule(ctx, tenantID, businessAppCode, graphKey); err != nil {
		return route, fmt.Errorf("resolve shadow: %w", err)
	} else if rule != nil && rule.TrafficPercent > 0 && sampleHit(runID, "shadow", rule.TrafficPercent) {
		route.ShadowGraphKey = rule.ShadowGraphKey
		route.ShadowRuleID = rule.ID
	}
	return route, nil
}

// RecordShadowExecution 记录主 Run ↔ 影子 Run 关联(Gateway best-effort 调用)。
func (r *Router) RecordShadowExecution(ctx context.Context, tenantID, ruleID, primaryRunID, shadowRunID string) error {
	if r.repo == nil {
		return nil
	}
	return r.repo.InsertShadowExecution(ctx, &ShadowExecution{
		TenantID: tenantID, RuleID: ruleID, PrimaryRunID: primaryRunID, ShadowRunID: shadowRunID,
	})
}

// CurrentStagePercent 当前阶梯的流量百分比(越界索引安全收敛为 0)。
func (c *CanaryRelease) CurrentStagePercent() int {
	stages := c.Stages()
	if c.CurrentStageIndex < 0 || c.CurrentStageIndex >= len(stages) {
		return 0
	}
	return stages[c.CurrentStageIndex]
}

// sampleHit FNV-1a 确定性哈希取样: hash(runID+salt) % 100 < percent。
// 同一 run_id 在所有进程/重试中结果一致,保证切分稳定。
func sampleHit(runID, salt string, percent int) bool {
	h := fnv.New32a()
	_, _ = h.Write([]byte(runID + ":" + salt))
	return int(h.Sum32()%100) < percent
}
