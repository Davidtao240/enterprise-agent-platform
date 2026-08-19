package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sort"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/agent"
	"github.com/google/uuid"
)

// RunStarter 实验 Run 启动契约(由 *agent.Gateway 实现;测试用替身)。
type RunStarter interface {
	StartExperimentRun(ctx context.Context, payload *agent.AgentRunPayload, graphKey string, metadata map[string]any) (*agent.RuntimeV2AcceptedResponse, error)
}

// Service Replay / Shadow / Canary 管理面业务逻辑。
type Service struct {
	repo   *Repository
	starter RunStarter
	// pollInterval/pollTimeout 为 Replay 终态轮询参数(测试可调)。
	pollInterval time.Duration
	pollTimeout  time.Duration
}

// NewService 创建 Service(starter 为 nil 时 Replay 发起将失败,Shadow/Canary 不受影响)。
func NewService(repo *Repository, starter RunStarter) *Service {
	return &Service{
		repo:         repo,
		starter:      starter,
		pollInterval: 2 * time.Second,
		pollTimeout:  runtimeMaxWait,
	}
}

// runtimeMaxWait Replay Run 终态轮询上限(经验值:预算 MaxSteps=30 的最坏耗时)。
const runtimeMaxWait = 10 * time.Minute

// ── Replay ──

// StartReplay 发起输入级回放: 创建 pending session 后异步执行,立即返回
// (Spec §4.5: 异步执行,返回 replay session)。
func (s *Service) StartReplay(ctx context.Context, tenantID string, req CreateReplayRequest, createdBy string) (*ReplaySession, error) {
	source, err := s.repo.FindSourceRun(ctx, tenantID, req.SourceRunID)
	if err != nil {
		return nil, fmt.Errorf("source run %s not available: %w", req.SourceRunID, err)
	}
	graphKey := req.GraphKey
	if graphKey == "" {
		graphKey = source.GraphKey
	}
	session := &ReplaySession{
		TenantID: tenantID, SourceRunID: source.ID,
		GraphKey: graphKey, Status: ReplayStatusPending, CreatedBy: createdBy,
	}
	if err := s.repo.CreateReplaySession(ctx, session); err != nil {
		return nil, fmt.Errorf("create replay session: %w", err)
	}
	go s.runReplayWithRecovery(session, source)
	return session, nil
}

// runReplayWithRecovery 包装 executeReplay 以捕获 panic,防止 goroutine 崩溃进程。
func (s *Service) runReplayWithRecovery(session *ReplaySession, source *SourceRun) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[experiment] replay session %s panic recovered: %v", session.ID, r)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			errJSON, _ := json.Marshal(map[string]any{"panic": fmt.Sprintf("%v", r)})
			errStr := string(errJSON)
			s.repo.UpdateReplaySession(ctx, session.TenantID, session.ID, ReplayStatusFailed, nil, &errStr)
		}
	}()
	s.executeReplay(session, source)
}

// GetReplay 查询回放状态与 Diff 报告。
func (s *Service) GetReplay(ctx context.Context, tenantID, id string) (*ReplaySession, error) {
	return s.repo.GetReplaySession(ctx, tenantID, id)
}

// executeReplay 回放执行闭环: running → 启动实验 Run → 轮询终态 → Diff → completed/failed。
// 全程异步,失败仅落 session 状态,不影响调用方。
func (s *Service) executeReplay(session *ReplaySession, source *SourceRun) {
	ctx, cancel := context.WithTimeout(context.Background(), s.pollTimeout+60*time.Second)
	defer cancel()

	fail := func(format string, args ...any) {
		errJSON, _ := json.Marshal(map[string]any{"error": fmt.Sprintf(format, args...)})
		errStr := string(errJSON)
		session.Status = ReplayStatusFailed
		session.DiffJSON = &errStr
		if err := s.repo.UpdateReplaySession(ctx, session.TenantID, session.ID, ReplayStatusFailed, nil, &errStr); err != nil {
			log.Printf("[experiment] replay session %s mark failed: %v", session.ID, err)
		}
	}

	inputJSON, err := s.repo.FindSourceRunInput(ctx, session.TenantID, session.SourceRunID)
	if err != nil {
		fail("extract source run input: %v", err)
		return
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		fail("parse source run input: %v", err)
		return
	}
	if s.starter == nil {
		fail("experiment run starter is not configured")
		return
	}
	payload := &agent.AgentRunPayload{
		TraceID: uuid.NewString(), // 回放使用新 trace,经 session/metadata 与源 Run 关联
		BusinessAppCode: source.BusinessAppCode,
		GraphKey:        session.GraphKey,
		ThreadTitle:     "replay:" + session.ID,
		Input:           input,
		UserID:          session.CreatedBy,
		TenantID:        session.TenantID,
	}
	metadata := map[string]any{
		"replay":            true,
		"source_run_id":     session.SourceRunID,
		"replay_session_id": session.ID,
	}
	accepted, err := s.starter.StartExperimentRun(ctx, payload, session.GraphKey, metadata)
	if err != nil {
		fail("start replay run: %v", err)
		return
	}
	replayRunID := accepted.RunID
	session.ReplayRunID = &replayRunID
	session.Status = ReplayStatusRunning
	if err := s.repo.UpdateReplaySession(ctx, session.TenantID, session.ID, ReplayStatusRunning, &replayRunID, nil); err != nil {
		log.Printf("[experiment] replay session %s mark running: %v", session.ID, err)
	}

	terminal, err := s.waitTerminal(ctx, session.TenantID, replayRunID)
	if err != nil {
		fail("wait replay run terminal: %v", err)
		return
	}
	diff := BuildDiffReport(source, terminal, replayRunID)
	diffJSON, err := json.Marshal(diff)
	if err != nil {
		fail("marshal diff: %v", err)
		return
	}
	diffStr := string(diffJSON)
	status := ReplayStatusCompleted
	if terminal.Status != agent.RunStatusSucceeded {
		status = ReplayStatusFailed
	}
	session.Status = status
	session.DiffJSON = &diffStr
	if err := s.repo.UpdateReplaySession(ctx, session.TenantID, session.ID, status, nil, &diffStr); err != nil {
		log.Printf("[experiment] replay session %s finalize: %v", session.ID, err)
	}
}

// waitTerminal 轮询实验 Run 直至终态或超时。
func (s *Service) waitTerminal(ctx context.Context, tenantID, runID string) (*RunTerminalState, error) {
	deadline := time.Now().Add(s.pollTimeout)
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		state, err := s.repo.GetRunTerminalState(ctx, tenantID, runID)
		if err != nil {
			return nil, err
		}
		switch state.Status {
		case agent.RunStatusSucceeded, agent.RunStatusFailed, agent.RunStatusCancelled:
			return state, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("replay run %s not terminal after %s (status=%s)", runID, s.pollTimeout, state.Status)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// FieldDiff 单字段对比(same / changed / added / removed)。
type FieldDiff struct {
	Source any    `json:"source,omitempty"`
	Replay any    `json:"replay,omitempty"`
	Status string `json:"status"`
}

// DiffReport 输出摘要字段级 Diff 报告(Spec §4.1)。
type DiffReport struct {
	SourceRunID string                `json:"source_run_id"`
	ReplayRunID string                `json:"replay_run_id"`
	SourceStatus string               `json:"source_status"`
	ReplayStatus string               `json:"replay_status"`
	Identical   bool                  `json:"identical"`
	Fields      map[string]FieldDiff  `json:"fields"`
}

// BuildDiffReport 对比源 Run 与回放 Run 的 output_summary_json(字段级)。
func BuildDiffReport(source *SourceRun, replay *RunTerminalState, replayRunID string) *DiffReport {
	report := &DiffReport{
		SourceRunID: source.ID, ReplayRunID: replayRunID,
		SourceStatus: source.Status, ReplayStatus: replay.Status,
		Fields: map[string]FieldDiff{},
	}
	src := parseSummary(source.OutputSummaryJSON)
	rep := parseSummary(replay.OutputSummaryJSON)
	keys := make(map[string]struct{}, len(src)+len(rep))
	for k := range src {
		keys[k] = struct{}{}
	}
	for k := range rep {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, key := range sorted {
		srcVal, inSrc := src[key]
		repVal, inRep := rep[key]
		switch {
		case inSrc && inRep:
			if reflect.DeepEqual(srcVal, repVal) {
				report.Fields[key] = FieldDiff{Source: srcVal, Replay: repVal, Status: "same"}
			} else {
				report.Fields[key] = FieldDiff{Source: srcVal, Replay: repVal, Status: "changed"}
			}
		case inSrc:
			report.Fields[key] = FieldDiff{Source: srcVal, Status: "removed"}
		default:
			report.Fields[key] = FieldDiff{Replay: repVal, Status: "added"}
		}
	}
	report.Identical = replay.Status == source.Status
	for _, f := range report.Fields {
		if f.Status != "same" {
			report.Identical = false
			break
		}
	}
	return report
}

func parseSummary(raw *string) map[string]any {
	if raw == nil || *raw == "" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(*raw), &m); err != nil {
		return map[string]any{}
	}
	return m
}

// ── Shadow Rules ──

func (s *Service) CreateShadowRule(ctx context.Context, tenantID string, req CreateShadowRuleRequest, createdBy string) (*ShadowRule, error) {
	if req.GraphKey == req.ShadowGraphKey {
		return nil, fmt.Errorf("shadow_graph_key must differ from baseline graph_key")
	}
	rule := &ShadowRule{
		TenantID: tenantID, BusinessAppCode: req.BusinessAppCode,
		GraphKey: req.GraphKey, ShadowGraphKey: req.ShadowGraphKey,
		TrafficPercent: req.TrafficPercent, CreatedBy: createdBy,
	}
	if err := s.repo.CreateShadowRule(ctx, rule); err != nil {
		return nil, fmt.Errorf("create shadow rule: %w", err)
	}
	return rule, nil
}

func (s *Service) ListShadowRules(ctx context.Context, tenantID string) ([]*ShadowRule, error) {
	return s.repo.ListShadowRules(ctx, tenantID)
}

func (s *Service) StopShadowRule(ctx context.Context, tenantID, id string) error {
	if err := s.repo.StopShadowRule(ctx, tenantID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("shadow rule %s not found or already stopped", id)
		}
		return err
	}
	return nil
}

func (s *Service) ListShadowExecutions(ctx context.Context, tenantID string, limit int) ([]*ShadowExecution, error) {
	return s.repo.ListShadowExecutions(ctx, tenantID, limit)
}

// ── Canary Releases ──

func (s *Service) CreateCanaryRelease(ctx context.Context, tenantID string, req CreateCanaryReleaseRequest, createdBy string) error {
	if req.GraphKey == req.CandidateGraphKey {
		return fmt.Errorf("candidate_graph_key must differ from baseline graph_key")
	}
	if !ascendingStages(req.Stages) {
		return fmt.Errorf("stages must be strictly ascending within 1..100 (e.g. [1,5,20,100])")
	}
	stagesJSON, _ := json.Marshal(req.Stages)
	release := &CanaryRelease{
		TenantID: tenantID, BusinessAppCode: req.BusinessAppCode,
		GraphKey: req.GraphKey, CandidateGraphKey: req.CandidateGraphKey,
		StagesJSON: string(stagesJSON), MaxErrorRate: req.MaxErrorRate,
		MinSampleSize: req.MinSampleSize, CreatedBy: createdBy,
	}
	return s.repo.CreateCanaryRelease(ctx, release)
}

func ascendingStages(stages []int) bool {
	if len(stages) < 2 {
		return false
	}
	for _, s := range stages {
		if s < 1 || s > 100 {
			return false
		}
	}
	for i := 1; i < len(stages); i++ {
		if stages[i] <= stages[i-1] {
			return false
		}
	}
	return true
}

func (s *Service) GetCanaryRelease(ctx context.Context, tenantID, id string) (*CanaryRelease, error) {
	release, err := s.repo.GetCanaryRelease(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if release == nil {
		return nil, ErrNotFound
	}
	return release, nil
}

func (s *Service) ListCanaryReleases(ctx context.Context, tenantID string) ([]*CanaryRelease, error) {
	return s.repo.ListCanaryReleases(ctx, tenantID)
}

// AdvanceCanary 人工确认进入下一阶梯(一次一级,不可跳级; Spec §4.6)。
func (s *Service) AdvanceCanary(ctx context.Context, tenantID, id string) (*CanaryRelease, error) {
	release, err := s.GetCanaryRelease(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if release.Status != CanaryStatusActive {
		return nil, fmt.Errorf("canary release %s is %s; only active releases can advance", id, release.Status)
	}
	stages := release.Stages()
	if release.CurrentStageIndex >= len(stages)-1 {
		return nil, fmt.Errorf("canary release %s already at final stage; use promote", id)
	}
	next := release.CurrentStageIndex + 1
	if err := s.repo.UpdateCanaryState(ctx, tenantID, id, next, CanaryStatusActive); err != nil {
		return nil, err
	}
	return s.GetCanaryRelease(ctx, tenantID, id)
}

// PromoteCanary 全量发布(等价于直接进入 100%; Spec §4.6)。
func (s *Service) PromoteCanary(ctx context.Context, tenantID, id string) (*CanaryRelease, error) {
	release, err := s.GetCanaryRelease(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if release.Status != CanaryStatusActive {
		return nil, fmt.Errorf("canary release %s is %s; only active releases can promote", id, release.Status)
	}
	last := len(release.Stages()) - 1
	if err := s.repo.UpdateCanaryState(ctx, tenantID, id, last, CanaryStatusPromoted); err != nil {
		return nil, err
	}
	return s.GetCanaryRelease(ctx, tenantID, id)
}

// RollbackCanary 人工回滚(切流立即回到基线版本)。
func (s *Service) RollbackCanary(ctx context.Context, tenantID, id string) (*CanaryRelease, error) {
	release, err := s.GetCanaryRelease(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if release.Status != CanaryStatusActive {
		return nil, fmt.Errorf("canary release %s is %s; only active releases can roll back", id, release.Status)
	}
	if err := s.repo.UpdateCanaryState(ctx, tenantID, id, release.CurrentStageIndex, CanaryStatusRolledBack); err != nil {
		return nil, err
	}
	return s.GetCanaryRelease(ctx, tenantID, id)
}

// CheckCanary 触发指标检查: 样本数 ≥ min_sample_size 且错误率 > max_error_rate
// 时自动回滚(两条件缺一不可; Spec §4.6)。统计窗口自当前阶梯生效时刻起。
func (s *Service) CheckCanary(ctx context.Context, tenantID, id string) (*CanaryCheckResult, error) {
	release, err := s.GetCanaryRelease(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if release.Status != CanaryStatusActive {
		return nil, fmt.Errorf("canary release %s is %s; only active releases can be checked", id, release.Status)
	}
	total, failed, err := s.repo.CanaryCandidateStats(ctx, tenantID, release.ID, release.CandidateGraphKey, release.UpdatedAt)
	if err != nil {
		return nil, err
	}
	result := &CanaryCheckResult{
		ReleaseID: release.ID, CandidateGraph: release.CandidateGraphKey,
		SampleSize: total, FailedRuns: failed,
		ErrorRate: 0, MaxErrorRate: release.MaxErrorRate,
	}
	if total > 0 {
		result.ErrorRate = float64(failed) / float64(total)
	}
	if total >= release.MinSampleSize && result.ErrorRate > release.MaxErrorRate {
		if err := s.repo.UpdateCanaryState(ctx, tenantID, id, release.CurrentStageIndex, CanaryStatusRolledBack); err != nil {
			return nil, err
		}
		result.RolledBack = true
	}
	return result, nil
}
