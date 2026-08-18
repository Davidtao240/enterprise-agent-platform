package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound 统一的租户内未找到错误。
var ErrNotFound = errors.New("not found")

// SourceRun Replay 输入提取所需的源 Run 画像(经 thread 补全 business_app_code)。
type SourceRun struct {
	ID               string
	TenantID         string
	BusinessAppCode  string
	GraphKey         string
	GraphVersion     string
	Status           string
	OutputSummaryJSON *string
}

// RunTerminalState 轮询实验 Run 终态所需的最小字段集。
type RunTerminalState struct {
	Status             string
	OutputSummaryJSON  *string
}

// Repository experiment 存储访问(所有查询强制 tenant_id 边界)。
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository 创建 Repository。
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// ── Replay Sessions ──

func (r *Repository) CreateReplaySession(ctx context.Context, s *ReplaySession) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO replay_sessions (tenant_id, source_run_id, graph_key, status, created_by)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, created_at, updated_at`,
		s.TenantID, s.SourceRunID, s.GraphKey, ReplayStatusPending, s.CreatedBy,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

func (r *Repository) UpdateReplaySession(ctx context.Context, tenantID, id string, status string, replayRunID *string, diffJSON *string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE replay_sessions
		 SET status = $3, updated_at = now(),
		     replay_run_id = COALESCE($4, replay_run_id),
		     diff_json = COALESCE($5, diff_json)
		 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id, status, replayRunID, diffJSON,
	)
	if err != nil {
		return fmt.Errorf("update replay session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) GetReplaySession(ctx context.Context, tenantID, id string) (*ReplaySession, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, source_run_id, replay_run_id, graph_key, status, diff_json, created_by, created_at, updated_at
		 FROM replay_sessions WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	)
	var s ReplaySession
	err := row.Scan(&s.ID, &s.TenantID, &s.SourceRunID, &s.ReplayRunID, &s.GraphKey, &s.Status, &s.DiffJSON, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get replay session: %w", err)
	}
	return &s, nil
}

// ── Shadow Rules / Executions ──

func (r *Repository) CreateShadowRule(ctx context.Context, rule *ShadowRule) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO shadow_rules (tenant_id, business_app_code, graph_key, shadow_graph_key, traffic_percent, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id, status, created_at, updated_at`,
		rule.TenantID, rule.BusinessAppCode, rule.GraphKey, rule.ShadowGraphKey, rule.TrafficPercent, rule.CreatedBy,
	).Scan(&rule.ID, &rule.Status, &rule.CreatedAt, &rule.UpdatedAt)
}

func (r *Repository) ListShadowRules(ctx context.Context, tenantID string) ([]*ShadowRule, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, business_app_code, graph_key, shadow_graph_key, traffic_percent, status, created_by, created_at, updated_at
		 FROM shadow_rules WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 200`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list shadow rules: %w", err)
	}
	defer rows.Close()
	var rules []*ShadowRule
	for rows.Next() {
		var rule ShadowRule
		if err := rows.Scan(&rule.ID, &rule.TenantID, &rule.BusinessAppCode, &rule.GraphKey, &rule.ShadowGraphKey, &rule.TrafficPercent, &rule.Status, &rule.CreatedBy, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan shadow rule: %w", err)
		}
		rules = append(rules, &rule)
	}
	return rules, rows.Err()
}

// StopShadowRule 仅 active 规则可停止(幂等: 已停止返回 ErrNotFound 提示状态冲突)。
func (r *Repository) StopShadowRule(ctx context.Context, tenantID, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE shadow_rules SET status = $3, updated_at = now()
		 WHERE tenant_id = $1 AND id = $2 AND status = $4`,
		tenantID, id, ShadowRuleStatusStopped, ShadowRuleStatusActive,
	)
	if err != nil {
		return fmt.Errorf("stop shadow rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FindActiveShadowRule 匹配唯一 active Shadow 规则(基线 graph 维度)。
func (r *Repository) FindActiveShadowRule(ctx context.Context, tenantID, businessAppCode, graphKey string) (*ShadowRule, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, business_app_code, graph_key, shadow_graph_key, traffic_percent, status, created_by, created_at, updated_at
		 FROM shadow_rules
		 WHERE tenant_id = $1 AND business_app_code = $2 AND graph_key = $3 AND status = 'active'
		 ORDER BY created_at DESC LIMIT 1`,
		tenantID, businessAppCode, graphKey,
	)
	var rule ShadowRule
	err := row.Scan(&rule.ID, &rule.TenantID, &rule.BusinessAppCode, &rule.GraphKey, &rule.ShadowGraphKey, &rule.TrafficPercent, &rule.Status, &rule.CreatedBy, &rule.CreatedAt, &rule.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find active shadow rule: %w", err)
	}
	return &rule, nil
}

func (r *Repository) InsertShadowExecution(ctx context.Context, exec *ShadowExecution) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO shadow_executions (tenant_id, rule_id, primary_run_id, shadow_run_id)
		 VALUES ($1,$2,$3,$4) RETURNING id, created_at`,
		exec.TenantID, exec.RuleID, exec.PrimaryRunID, exec.ShadowRunID,
	).Scan(&exec.ID, &exec.CreatedAt)
}

func (r *Repository) ListShadowExecutions(ctx context.Context, tenantID string, limit int) ([]*ShadowExecution, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, rule_id, primary_run_id, shadow_run_id, comparison_json, created_at
		 FROM shadow_executions WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2`,
		tenantID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list shadow executions: %w", err)
	}
	defer rows.Close()
	var execs []*ShadowExecution
	for rows.Next() {
		var e ShadowExecution
		if err := rows.Scan(&e.ID, &e.TenantID, &e.RuleID, &e.PrimaryRunID, &e.ShadowRunID, &e.ComparisonJSON, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan shadow execution: %w", err)
		}
		execs = append(execs, &e)
	}
	return execs, rows.Err()
}

// ── Canary Releases ──

func (r *Repository) CreateCanaryRelease(ctx context.Context, c *CanaryRelease) error {
	stagesJSON, err := json.Marshal(c.StagesJSON)
	if err != nil {
		return fmt.Errorf("marshal canary stages: %w", err)
	}
	return r.pool.QueryRow(ctx,
		`INSERT INTO canary_releases
			 (tenant_id, business_app_code, graph_key, candidate_graph_key, stages, current_stage_index,
			  max_error_rate, min_sample_size, created_by)
		 VALUES ($1,$2,$3,$4,$5,0,$6,$7,$8)
		 RETURNING id, status, current_stage_index, created_at, updated_at`,
		c.TenantID, c.BusinessAppCode, c.GraphKey, c.CandidateGraphKey, string(stagesJSON), c.MaxErrorRate, c.MinSampleSize, c.CreatedBy,
	).Scan(&c.ID, &c.Status, &c.CurrentStageIndex, &c.CreatedAt, &c.UpdatedAt)
}

func (r *Repository) GetCanaryRelease(ctx context.Context, tenantID, id string) (*CanaryRelease, error) {
	return r.scanCanary(r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, business_app_code, graph_key, candidate_graph_key, stages::text,
		        current_stage_index, max_error_rate, min_sample_size, status, created_by, created_at, updated_at
		 FROM canary_releases WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	))
}

func (r *Repository) ListCanaryReleases(ctx context.Context, tenantID string) ([]*CanaryRelease, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, business_app_code, graph_key, candidate_graph_key, stages::text,
		        current_stage_index, max_error_rate, min_sample_size, status, created_by, created_at, updated_at
		 FROM canary_releases WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 200`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list canary releases: %w", err)
	}
	defer rows.Close()
	var releases []*CanaryRelease
	for rows.Next() {
		c, err := r.scanCanary(rows)
		if err != nil {
			return nil, err
		}
		releases = append(releases, c)
	}
	return releases, rows.Err()
}

// FindActiveCanaryRelease 匹配基线 graph 维度的 active Canary 发布。
func (r *Repository) FindActiveCanaryRelease(ctx context.Context, tenantID, businessAppCode, graphKey string) (*CanaryRelease, error) {
	return r.scanCanary(r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, business_app_code, graph_key, candidate_graph_key, stages::text,
		        current_stage_index, max_error_rate, min_sample_size, status, created_by, created_at, updated_at
		 FROM canary_releases
		 WHERE tenant_id = $1 AND business_app_code = $2 AND graph_key = $3 AND status = 'active'
		 ORDER BY created_at DESC LIMIT 1`,
		tenantID, businessAppCode, graphKey,
	))
}

// UpdateCanaryState 原子更新阶梯与状态(advance/promote/rollback 共用)。
func (r *Repository) UpdateCanaryState(ctx context.Context, tenantID, id string, stageIndex int, status string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE canary_releases
		 SET current_stage_index = $3, status = $4, updated_at = now()
		 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id, stageIndex, status,
	)
	if err != nil {
		return fmt.Errorf("update canary state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CanaryCandidateStats 统计发布窗口内命中候选版本的 Run(经 metadata 标记关联)。
func (r *Repository) CanaryCandidateStats(ctx context.Context, tenantID, releaseID, candidateGraphKey string, since time.Time) (total, failed int, err error) {
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'failed')
		 FROM agent_runs
		 WHERE tenant_id = $1 AND graph_key = $2
		   AND metadata_json->>'canary_release_id' = $3
		   AND finished_at IS NOT NULL AND finished_at >= $4`,
		tenantID, candidateGraphKey, releaseID, since,
	).Scan(&total, &failed)
	if err != nil {
		return 0, 0, fmt.Errorf("canary candidate stats: %w", err)
	}
	return total, failed, nil
}

type canaryRow interface{ Scan(dest ...any) error }

func (r *Repository) scanCanary(row canaryRow) (*CanaryRelease, error) {
	var c CanaryRelease
	err := row.Scan(&c.ID, &c.TenantID, &c.BusinessAppCode, &c.GraphKey, &c.CandidateGraphKey, &c.StagesJSON,
		&c.CurrentStageIndex, &c.MaxErrorRate, &c.MinSampleSize, &c.Status, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan canary release: %w", err)
	}
	return &c, nil
}

// ── Replay 源数据提取(只读跨表查询) ──

// FindSourceRun 定位源 Run 并经 thread 补全 business_app_code。
func (r *Repository) FindSourceRun(ctx context.Context, tenantID, runID string) (*SourceRun, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT ar.id, ar.tenant_id, COALESCE(t.business_app_code, ''), ar.graph_key, ar.graph_version,
		        ar.status, ar.output_summary_json::text
		 FROM agent_runs ar
		 JOIN agent_threads t ON t.id = ar.thread_id
		 WHERE ar.tenant_id = $1 AND ar.id = $2`,
		tenantID, runID,
	)
	var s SourceRun
	err := row.Scan(&s.ID, &s.TenantID, &s.BusinessAppCode, &s.GraphKey, &s.GraphVersion, &s.Status, &s.OutputSummaryJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find source run: %w", err)
	}
	return &s, nil
}

// FindSourceRunInput 源 Run 首个 model Step 输入;缺失时回退 agent_run_logs 输入摘要
// (Spec §4.1 MVP: 输入级回放)。
func (r *Repository) FindSourceRunInput(ctx context.Context, tenantID, runID string) (string, error) {
	var input string
	err := r.pool.QueryRow(ctx,
		`SELECT input_summary_json::text FROM agent_run_steps
		 WHERE tenant_id = $1 AND run_id = $2 AND step_type = 'model' AND input_summary_json IS NOT NULL
		 ORDER BY sequence ASC LIMIT 1`,
		tenantID, runID,
	).Scan(&input)
	if errors.Is(err, pgx.ErrNoRows) {
		err = r.pool.QueryRow(ctx,
			`SELECT input_summary_json::text FROM agent_run_logs
			 WHERE tenant_id = $1 AND durable_run_id = $2 AND input_summary_json IS NOT NULL
			 ORDER BY created_at ASC LIMIT 1`,
			tenantID, runID,
		).Scan(&input)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("find source run input: %w", err)
	}
	return input, nil
}

// GetRunTerminalState 轮询实验 Run 的终态与输出(Replay Diff 数据源)。
func (r *Repository) GetRunTerminalState(ctx context.Context, tenantID, runID string) (*RunTerminalState, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT status, output_summary_json::text FROM agent_runs WHERE tenant_id = $1 AND id = $2`,
		tenantID, runID,
	)
	var s RunTerminalState
	err := row.Scan(&s.Status, &s.OutputSummaryJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get run terminal state: %w", err)
	}
	return &s, nil
}
