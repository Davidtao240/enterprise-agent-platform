package tool

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── M3-C: Connector Outbox 持久化 ──
//
// Saga/Outbox/Compensation 的存储层:
//   - 写请求先落库(pending),Dispatcher 周期投递 —— 与业务事务解耦;
//   - attempts/next_attempt_at 实现指数退避重试;
//   - tool_call_id 关联 ToolCall,经 trace_id 串联 Run/Step/Audit(全链路关联);
//   - 状态只前进:pending → sent → confirmed / compensate_pending → compensated / failed。

// Outbox 状态机。
const (
	OutboxStatePending           = "pending"           // 待投递
	OutboxStateSent              = "sent"              // 已发出,待 Verify 确认
	OutboxStateConfirmed         = "confirmed"         // 外部副作用已核验(终态)
	OutboxStateCompensatePending = "compensate_pending" // 待补偿
	OutboxStateCompensated       = "compensated"        // 补偿完成(终态)
	OutboxStateFailed            = "failed"             // 不可恢复失败(终态)
)

// ErrOutboxNotFound Outbox 记录不存在。
var ErrOutboxNotFound = errors.New("outbox entry not found")

// ErrOutboxStateConflict 非法状态迁移(终态记录不可再变)。
var ErrOutboxStateConflict = errors.New("outbox entry in terminal state")

// OutboxEntry 对应 connector_outbox 表。
type OutboxEntry struct {
	ID                string     `json:"id"`
	ToolCallID        string     `json:"tool_call_id"`
	TenantID          string     `json:"tenant_id"`
	ConnectorCode     string     `json:"connector_code"`
	Operation         string     `json:"operation"`
	PayloadJSON       string     `json:"payload_json"`
	State             string     `json:"state"`
	ExternalRequestID *string    `json:"external_request_id,omitempty"`
	ExternalObjectID  *string    `json:"external_object_id,omitempty"`
	Attempts          int        `json:"attempts"`
	NextAttemptAt     time.Time  `json:"next_attempt_at"`
	LastError         *string    `json:"last_error,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// OutboxEntryRepository 封装 connector_outbox 表。
type OutboxEntryRepository struct {
	pool *pgxpool.Pool
}

// NewOutboxEntryRepository 创建仓储。
func NewOutboxEntryRepository(pool *pgxpool.Pool) *OutboxEntryRepository {
	return &OutboxEntryRepository{pool: pool}
}

// Enqueue 落库一条待投递记录(Tool Gateway 执行前调用)。
func (r *OutboxEntryRepository) Enqueue(ctx context.Context, e *OutboxEntry) (*OutboxEntry, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO connector_outbox
			 (id, tool_call_id, tenant_id, connector_code, operation, payload_json,
			  state, next_attempt_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,'pending',$7,$8,$8)`,
		id, e.ToolCallID, e.TenantID, e.ConnectorCode, e.Operation, e.PayloadJSON, now, now)
	if err != nil {
		return nil, err
	}
	e.ID = id
	e.State = OutboxStatePending
	e.NextAttemptAt = now
	e.CreatedAt = now
	e.UpdatedAt = now
	return e, nil
}

// GetByID 按 ID 查找。
func (r *OutboxEntryRepository) GetByID(ctx context.Context, id string) (*OutboxEntry, error) {
	row := r.pool.QueryRow(ctx, outboxSelect+" WHERE id = $1", id)
	return scanOutboxEntry(row)
}

// FindByToolCallID 按关联 ToolCall 查找(全链路关联入口)。
func (r *OutboxEntryRepository) FindByToolCallID(ctx context.Context, toolCallID string) ([]*OutboxEntry, error) {
	return r.queryList(ctx, outboxSelect+" WHERE tool_call_id = $1 ORDER BY created_at", toolCallID)
}

// ListDispatchable 待投递且到期(next_attempt_at <= now)的记录。
func (r *OutboxEntryRepository) ListDispatchable(ctx context.Context, now time.Time, limit int) ([]*OutboxEntry, error) {
	return r.queryList(ctx,
		outboxSelect+" WHERE state = 'pending' AND next_attempt_at <= $1 ORDER BY next_attempt_at LIMIT $2",
		now, limit)
}

// ListStaleSent 已发出但超过 confirmTimeout 未确认的记录(Verify 收敛候选)。
// staleBefore = now - confirmTimeout,由调用方计算传入。
func (r *OutboxEntryRepository) ListStaleSent(ctx context.Context, staleBefore time.Time, limit int) ([]*OutboxEntry, error) {
	return r.queryList(ctx,
		outboxSelect+" WHERE state = 'sent' AND updated_at < $1 ORDER BY updated_at LIMIT $2",
		staleBefore, limit)
}

// ListCompensatable 待补偿且退避到期的记录。
func (r *OutboxEntryRepository) ListCompensatable(ctx context.Context, now time.Time, limit int) ([]*OutboxEntry, error) {
	return r.queryList(ctx,
		outboxSelect+" WHERE state = 'compensate_pending' AND next_attempt_at <= $1 ORDER BY next_attempt_at LIMIT $2",
		now, limit)
}

// MarkSent 投递成功:记录外部请求/对象 ID,转 sent。
func (r *OutboxEntryRepository) MarkSent(ctx context.Context, id, externalRequestID, externalObjectID string) error {
	return r.execState(ctx, id, "pending", `UPDATE connector_outbox
		SET state = 'sent', external_request_id = $2, external_object_id = $3, updated_at = now()
		WHERE id = $1 AND state = 'pending'`, id, externalRequestID, externalObjectID)
}

// MarkConfirmed Verify 核验通过,转 confirmed(终态)。
func (r *OutboxEntryRepository) MarkConfirmed(ctx context.Context, id string) error {
	return r.execState(ctx, id, "sent", `UPDATE connector_outbox
		SET state = 'confirmed', updated_at = now()
		WHERE id = $1 AND state = 'sent'`, id)
}

// MarkAttemptFailed 记录一次失败尝试并安排退避(保持 pending,守卫:终态不可更新)。
func (r *OutboxEntryRepository) MarkAttemptFailed(ctx context.Context, id string, attempts int, nextAttemptAt time.Time, lastErr string) error {
	ct, err := r.pool.Exec(ctx, `UPDATE connector_outbox
		SET attempts = $2, next_attempt_at = $3, last_error = $4, updated_at = now()
		WHERE id = $1 AND state NOT IN ('confirmed', 'compensated', 'failed')`, id, attempts, nextAttemptAt, lastErr)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrOutboxStateConflict
	}
	return nil
}

// MarkCompensatePending 转入待补偿(ToolCall 取消/重试耗尽且有副作用/人工触发)。
// 带守卫:终态(confirmed/compensated/failed)拒绝迁移。
// tenantID 为空时不过滤(internal 端点已在信任边界内);非空时强租户隔离。
func (r *OutboxEntryRepository) MarkCompensatePending(ctx context.Context, id, tenantID, reason string) error {
	var ct pgconn.CommandTag
	var err error
	if tenantID != "" {
		ct, err = r.pool.Exec(ctx, `UPDATE connector_outbox
			SET state = 'compensate_pending', last_error = $3, next_attempt_at = now(), updated_at = now()
			WHERE id = $1 AND tenant_id = $2 AND state IN ('pending', 'sent', 'compensate_pending')`,
			id, tenantID, reason)
	} else {
		ct, err = r.pool.Exec(ctx, `UPDATE connector_outbox
			SET state = 'compensate_pending', last_error = $2, next_attempt_at = now(), updated_at = now()
			WHERE id = $1 AND state IN ('pending', 'sent', 'compensate_pending')`, id, reason)
	}
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrOutboxStateConflict
	}
	return nil
}

// MarkCompensated 补偿完成(终态)。
func (r *OutboxEntryRepository) MarkCompensated(ctx context.Context, id string) error {
	return r.execState(ctx, id, "compensate_pending", `UPDATE connector_outbox
		SET state = 'compensated', updated_at = now()
		WHERE id = $1 AND state = 'compensate_pending'`, id)
}

// MarkFailedFinal 不可恢复失败(终态),last_error 留痕。
func (r *OutboxEntryRepository) MarkFailedFinal(ctx context.Context, id, code string) error {
	_, err := r.pool.Exec(ctx, `UPDATE connector_outbox
		SET state = 'failed', last_error = $2, updated_at = now()
		WHERE id = $1 AND state NOT IN ('confirmed', 'compensated', 'failed')`, id, code)
	return err
}

// Touch 刷新 updated_at(Verify 未决时延长观察窗口)。
func (r *OutboxEntryRepository) Touch(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE connector_outbox SET updated_at = now() WHERE id = $1`, id)
	return err
}

// ── 内部 ──

const outboxSelect = `SELECT id, tool_call_id, tenant_id, connector_code, operation, payload_json,
	state, external_request_id, external_object_id, attempts, next_attempt_at, last_error,
	created_at, updated_at FROM connector_outbox`

func (r *OutboxEntryRepository) queryList(ctx context.Context, sql string, args ...any) ([]*OutboxEntry, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*OutboxEntry
	for rows.Next() {
		e, err := scanOutboxEntry(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// execState 带状态守卫的迁移:期望状态未命中视为冲突。
func (r *OutboxEntryRepository) execState(ctx context.Context, id, expectedState, sql string, args ...any) error {
	ct, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrOutboxStateConflict
	}
	return nil
}

func scanOutboxEntry(row pgx.Row) (*OutboxEntry, error) {
	var e OutboxEntry
	err := row.Scan(
		&e.ID, &e.ToolCallID, &e.TenantID, &e.ConnectorCode, &e.Operation, &e.PayloadJSON,
		&e.State, &e.ExternalRequestID, &e.ExternalObjectID, &e.Attempts, &e.NextAttemptAt, &e.LastError,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOutboxNotFound
		}
		return nil, err
	}
	return &e, nil
}
