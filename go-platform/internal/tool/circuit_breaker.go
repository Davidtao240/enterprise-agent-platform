package tool

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── M2-C.9: Tool 熔断器 ──
//
// 熔断对象是外部系统(tool 维度),不是平台内部错误:
//
//	closed ──连续失败≥阈值──→ open ──冷却期满──→ half_open(单探测)
//	   ↑                                        │
//	   └────探测成功──── closed   探测失败→ open ─┘
//
// open 状态下 Execute 直接拒绝(CIRCUIT_OPEN),不产生新的外部副作用;
// indeterminate 不计入失败(结果未知,可能是网络问题而非外部系统故障)。

// ErrCircuitOpen 工具熔断中,拒绝新调用。
var ErrCircuitOpen = errors.New("tool circuit breaker is open")

// CircuitBreakerState 熔断器持久化状态。
type CircuitBreakerState struct {
	ToolID              string     `json:"tool_id"`
	State               string     `json:"state"` // closed / open / half_open
	ConsecutiveFailures int        `json:"consecutive_failures"`
	OpenedAt            *time.Time `json:"opened_at,omitempty"`
	HalfOpenProbeAt     *time.Time `json:"half_open_probe_at,omitempty"`
	LastFailureAt       *time.Time `json:"last_failure_at,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
}

// CircuitBreakerRepository 熔断状态表访问(原子 SQL 保证多实例安全)。
type CircuitBreakerRepository struct {
	pool *pgxpool.Pool
}

// NewCircuitBreakerRepository 创建熔断仓储。
func NewCircuitBreakerRepository(pool *pgxpool.Pool) *CircuitBreakerRepository {
	return &CircuitBreakerRepository{pool: pool}
}

// GetState 读取熔断状态;无记录视为 closed。
func (r *CircuitBreakerRepository) GetState(ctx context.Context, toolID string) (*CircuitBreakerState, error) {
	st := &CircuitBreakerState{ToolID: toolID, State: "closed"}
	err := r.pool.QueryRow(ctx,
		`SELECT state, consecutive_failures, opened_at, half_open_probe_at, last_failure_at, last_success_at
		 FROM tool_circuit_breakers WHERE tool_id = $1`, toolID,
	).Scan(&st.State, &st.ConsecutiveFailures, &st.OpenedAt, &st.HalfOpenProbeAt, &st.LastFailureAt, &st.LastSuccessAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	return st, nil
}

// RecordFailure 记录一次失败(原子):累加计数,达到阈值则熔断(open)。
// 条件说明:
//   - half_open 探测失败:计数重置为 1,状态立即 open
//   - closed 且累计失败≥阈值:计数 +1,状态转为 open
//   - closed 但未达阈值:计数 +1,状态保持 closed
//   - open 状态下不应调用(已被 Allow 拦截),若到达则重设计数为 1。
func (r *CircuitBreakerRepository) RecordFailure(ctx context.Context, toolID string, threshold int) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tool_circuit_breakers (id, tool_id, state, consecutive_failures, last_failure_at)
		 VALUES ($1, $2, 'closed', 1, $3)
		 ON CONFLICT (tool_id) DO UPDATE SET
		   consecutive_failures = CASE
		     WHEN tool_circuit_breakers.state = 'closed' THEN tool_circuit_breakers.consecutive_failures + 1
		     ELSE 1
		   END,
		   state = CASE
		     WHEN tool_circuit_breakers.state = 'half_open'
		       OR (tool_circuit_breakers.state = 'closed'
		           AND tool_circuit_breakers.consecutive_failures + 1 >= $4)
		     THEN 'open' ELSE tool_circuit_breakers.state
		   END,
		   opened_at = CASE
		     WHEN tool_circuit_breakers.state = 'half_open'
		       OR (tool_circuit_breakers.state = 'closed'
		           AND tool_circuit_breakers.consecutive_failures + 1 >= $4)
		     THEN $3 ELSE tool_circuit_breakers.opened_at
		   END,
		   half_open_probe_at = NULL,
		   last_failure_at = $3,
		   updated_at = $3`,
		uuid.NewString(), toolID, now, threshold)
	return err
}

// RecordSuccess 记录一次成功:重置为 closed。
func (r *CircuitBreakerRepository) RecordSuccess(ctx context.Context, toolID string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tool_circuit_breakers (id, tool_id, state, consecutive_failures, last_success_at)
		 VALUES ($1, $2, 'closed', 0, $3)
		 ON CONFLICT (tool_id) DO UPDATE SET
		   state = 'closed', consecutive_failures = 0,
		   opened_at = NULL, half_open_probe_at = NULL,
		   last_success_at = $3, updated_at = $3`,
		uuid.NewString(), toolID, now)
	return err
}

// TryEnterHalfOpen 冷却期满后 open → half_open(原子,仅放行一个探测者)。
// 返回 true 表示本次调用即为探测调用。
func (r *CircuitBreakerRepository) TryEnterHalfOpen(ctx context.Context, toolID string, cooldown time.Duration) (bool, error) {
	now := time.Now().UTC()
	tag, err := r.pool.Exec(ctx,
		`UPDATE tool_circuit_breakers
		 SET state = 'half_open', half_open_probe_at = $2, updated_at = $2
		 WHERE tool_id = $1 AND state = 'open' AND opened_at IS NOT NULL
		   AND opened_at + $3::interval <= $2`,
		toolID, now, cooldown.String())
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// CircuitBreaker 熔断判定器(组合 repository,供 Service 调用)。
type CircuitBreaker struct {
	repo      *CircuitBreakerRepository
	threshold int
	cooldown  time.Duration

	mu sync.Mutex // 半开探测互斥(单进程内;跨进程由 DB 原子性兜底)
}

// NewCircuitBreaker 创建熔断判定器。
func NewCircuitBreaker(repo *CircuitBreakerRepository, threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{repo: repo, threshold: threshold, cooldown: cooldown}
}

// Allow 检查工具是否放行新调用。
// 返回 (是否放行, 当前状态, error)。
// 对于 open 状态,先尝试原子进入 half_open(冷却期满后放行单探测)。
func (cb *CircuitBreaker) Allow(ctx context.Context, toolID string) (bool, string, error) {
	st, err := cb.repo.GetState(ctx, toolID)
	if err != nil {
		log.Printf("[circuit] get state for tool %s failed: %v (fail-open)", toolID, err)
		return true, "closed", nil
	}
	switch st.State {
	case "closed":
		return true, st.State, nil
	case "open":
		// 冷却期满 → 半开放行单探测
		// 使用内存锁减少同进程并发竞争,DB 原子操作兜底跨进程一致性
		cb.mu.Lock()
		defer cb.mu.Unlock()
		probe, err := cb.repo.TryEnterHalfOpen(ctx, toolID, cb.cooldown)
		if err != nil {
			log.Printf("[circuit] try enter half_open for tool %s failed: %v (fail-open)", toolID, err)
			return true, "open", nil
		}
		if probe {
			log.Printf("[circuit] tool %s: open → half_open (probe released)", toolID)
			return true, "half_open", nil
		}
		log.Printf("[circuit] tool %s: blocked by open state", toolID)
		return false, "open", nil
	case "half_open":
		// 探测已在途:拒绝并发,避免半开窗口被打穿
		log.Printf("[circuit] tool %s: blocked by half_open state (probe in flight)", toolID)
		return false, "half_open", nil
	default:
		return true, st.State, nil
	}
}

// RecordResult 记录执行结果驱动熔断状态机。
// outcome: "succeeded" / "failed"(indeterminate 不计入,外部结果未知)。
func (cb *CircuitBreaker) RecordResult(ctx context.Context, toolID, outcome string) {
	if cb == nil || cb.repo == nil {
		return
	}
	var err error
	switch outcome {
	case "succeeded":
		err = cb.repo.RecordSuccess(ctx, toolID)
	case "failed":
		err = cb.repo.RecordFailure(ctx, toolID, cb.threshold)
	default:
		return // indeterminate/其他不计入
	}
	if err != nil {
		log.Printf("[circuit] record %s for tool %s failed: %v", outcome, toolID, err)
	}
}

// CheckCircuitStateError 熔断拒绝的错误包装(带状态)。
func CheckCircuitStateError(state string) error {
	return fmt.Errorf("%w (state=%s)", ErrCircuitOpen, state)
}
