// Package experiment 实现 M5-C: Replay / Shadow / Canary 实验机制。
//
// Spec: docs/03_PLATFORM_SPEC/TRACE_AND_EVAL.md §4
//   - Replay: 输入级回放(源 Run 输入 → 独立实验 Run → 字段级 Diff);
//   - Shadow: 生产流量按规则异步复制到影子版本(不影响生产结果);
//   - Canary: 阶梯切流 + 指标检查 + 自动/人工回滚。
//
// 依赖方向: experiment → agent(Gateway 的实验 Run 启动契约);
// agent 包仅定义消费接口 ExperimentRouter,不反向依赖本包。
package experiment

import (
	"encoding/json"
	"time"
)

// ── Replay Session ──

// ReplaySession 状态机: pending → running → completed / failed (Spec §4.4)。
const (
	ReplayStatusPending   = "pending"
	ReplayStatusRunning   = "running"
	ReplayStatusCompleted = "completed"
	ReplayStatusFailed    = "failed"
)

type ReplaySession struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"tenant_id"`
	SourceRunID  string     `json:"source_run_id"`
	ReplayRunID  *string    `json:"replay_run_id,omitempty"`
	GraphKey     string     `json:"graph_key"`
	Status       string     `json:"status"`
	DiffJSON     *string    `json:"diff_json,omitempty"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CreateReplayRequest POST /api/v1/replays 请求体(Spec §4.5)。
type CreateReplayRequest struct {
	SourceRunID string `json:"source_run_id" binding:"required"`
	// GraphKey 可选;缺省使用源 Run 的 graph_key(指定不同 graph 用于算法对比)。
	GraphKey string `json:"graph_key"`
}

// ── Shadow Rule / Execution ──

const (
	ShadowRuleStatusActive  = "active"
	ShadowRuleStatusStopped = "stopped"
)

type ShadowRule struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	BusinessAppCode string   `json:"business_app_code"`
	GraphKey       string    `json:"graph_key"`
	ShadowGraphKey string    `json:"shadow_graph_key"`
	TrafficPercent int       `json:"traffic_percent"`
	Status         string    `json:"status"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateShadowRuleRequest POST /api/v1/shadow-rules 请求体。
type CreateShadowRuleRequest struct {
	BusinessAppCode string `json:"business_app_code" binding:"required"`
	GraphKey        string `json:"graph_key" binding:"required"`
	ShadowGraphKey  string `json:"shadow_graph_key" binding:"required"`
	TrafficPercent  int    `json:"traffic_percent" binding:"required,min=0,max=100"`
}

type ShadowExecution struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	RuleID         string    `json:"rule_id"`
	PrimaryRunID   string    `json:"primary_run_id"`
	ShadowRunID    string    `json:"shadow_run_id"`
	ComparisonJSON *string   `json:"comparison_json,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// ── Canary Release ──

// CanaryRelease 状态机: active → promoted / rolled_back (Spec §4.4)。
const (
	CanaryStatusActive     = "active"
	CanaryStatusPromoted   = "promoted"
	CanaryStatusRolledBack = "rolled_back"
)

type CanaryRelease struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id"`
	BusinessAppCode    string    `json:"business_app_code"`
	GraphKey           string    `json:"graph_key"`
	CandidateGraphKey  string    `json:"candidate_graph_key"`
	StagesJSON         string    `json:"stages"`          // 如 "[1,5,20,100]"
	CurrentStageIndex  int       `json:"current_stage_index"`
	MaxErrorRate       float64   `json:"max_error_rate"`
	MinSampleSize      int       `json:"min_sample_size"`
	Status             string    `json:"status"`
	CreatedBy          string    `json:"created_by"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Stages 解析 stages JSONB 为阶梯数组。
func (c *CanaryRelease) Stages() []int {
	var stages []int
	_ = json.Unmarshal([]byte(c.StagesJSON), &stages)
	return stages
}

// CreateCanaryReleaseRequest POST /api/v1/canary-releases 请求体。
type CreateCanaryReleaseRequest struct {
	BusinessAppCode   string  `json:"business_app_code" binding:"required"`
	GraphKey          string  `json:"graph_key" binding:"required"`
	CandidateGraphKey string  `json:"candidate_graph_key" binding:"required"`
	Stages            []int   `json:"stages" binding:"required,min=1,dive,min=1,max=100"`
	MaxErrorRate      float64 `json:"max_error_rate" binding:"required,gt=0,lte=1"`
	MinSampleSize     int     `json:"min_sample_size" binding:"required,min=1"`
}

// CanaryCheckResult POST /canary-releases/:id/check 响应。
type CanaryCheckResult struct {
	ReleaseID      string  `json:"release_id"`
	CandidateGraph string  `json:"candidate_graph_key"`
	SampleSize     int     `json:"sample_size"`
	FailedRuns     int     `json:"failed_runs"`
	ErrorRate      float64 `json:"error_rate"`
	MaxErrorRate   float64 `json:"max_error_rate"`
	RolledBack     bool    `json:"rolled_back"` // true = 本次检查触发自动回滚
}
