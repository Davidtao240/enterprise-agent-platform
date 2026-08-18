package tool

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── M3-B: Webhook Inbox 持久化 ──
//
// 外部系统事件先落库后消费(Inbox 模式):
//   - 投递去重:(connector_code, external_event_id) 唯一约束,重复投递幂等返回;
//   - 签名结果持久化:signature_valid=false 的事件落库但不进入处理;
//   - 消费状态:processed_at 为空表示待处理,process_error 记录最近失败。

// ErrWebhookDuplicate 重复投递(唯一约束命中,幂等场景非错误)。
var ErrWebhookDuplicate = errors.New("webhook event already ingested")

// ErrWebhookNotFound 事件不存在。
var ErrWebhookNotFound = errors.New("webhook event not found")

// WebhookEvent 对应 webhook_events 表。
type WebhookEvent struct {
	ID              string     `json:"id"`
	ConnectorCode   string     `json:"connector_code"`
	ExternalEventID string     `json:"external_event_id"`
	SignatureValid  bool       `json:"signature_valid"`
	PayloadJSON     string     `json:"payload_json"`
	ReceivedAt      time.Time  `json:"received_at"`
	ProcessedAt     *time.Time `json:"processed_at,omitempty"`
	ProcessError    *string    `json:"process_error,omitempty"`
}

// WebhookEventRepository 封装 webhook_events 表 CRUD。
type WebhookEventRepository struct {
	pool *pgxpool.Pool
}

// NewWebhookEventRepository 创建 WebhookEventRepository。
func NewWebhookEventRepository(pool *pgxpool.Pool) *WebhookEventRepository {
	return &WebhookEventRepository{pool: pool}
}

// Ingest 落库一条事件。重复投递(同 connector+external_event_id)
// 返回已存在记录与 ErrWebhookDuplicate(调用方按幂等处理)。
func (r *WebhookEventRepository) Ingest(ctx context.Context, e *WebhookEvent) (*WebhookEvent, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	row := r.pool.QueryRow(ctx,
		`INSERT INTO webhook_events
			 (id, connector_code, external_event_id, signature_valid, payload_json, received_at)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (connector_code, external_event_id) DO NOTHING
		 RETURNING id`,
		id, e.ConnectorCode, e.ExternalEventID, e.SignatureValid, e.PayloadJSON, now)
	var insertedID string
	if err := row.Scan(&insertedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// 冲突:取已有记录返回(幂等)
			existing, getErr := r.FindByExternalID(ctx, e.ConnectorCode, e.ExternalEventID)
			if getErr != nil {
				return nil, getErr
			}
			return existing, ErrWebhookDuplicate
		}
		return nil, err
	}
	return &WebhookEvent{
		ID:              insertedID,
		ConnectorCode:   e.ConnectorCode,
		ExternalEventID: e.ExternalEventID,
		SignatureValid:  e.SignatureValid,
		PayloadJSON:     e.PayloadJSON,
		ReceivedAt:      now,
	}, nil
}

// FindByExternalID 按去重键查找。
func (r *WebhookEventRepository) FindByExternalID(ctx context.Context, connectorCode, externalEventID string) (*WebhookEvent, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, connector_code, external_event_id, signature_valid, payload_json,
		        received_at, processed_at, process_error
		 FROM webhook_events
		 WHERE connector_code = $1 AND external_event_id = $2`,
		connectorCode, externalEventID)
	return scanWebhookEvent(row)
}

// ListUnprocessed 列出待处理事件(含此前处理失败待重试的),
// 按 received_at 排序保证消费顺序稳定(乱序恢复由消费端 occurred_at 判定)。
func (r *WebhookEventRepository) ListUnprocessed(ctx context.Context, limit int) ([]*WebhookEvent, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, connector_code, external_event_id, signature_valid, payload_json,
		        received_at, processed_at, process_error
		 FROM webhook_events
		 WHERE processed_at IS NULL AND signature_valid = TRUE
		 ORDER BY received_at
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*WebhookEvent
	for rows.Next() {
		e, err := scanWebhookEvent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, nil
}

// MarkProcessed 标记消费完成。
func (r *WebhookEventRepository) MarkProcessed(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE webhook_events
		 SET processed_at = NOW(), process_error = NULL, updated_at = NOW()
		 WHERE id = $1`, id)
	return err
}

// MarkProcessError 记录处理失败(保留待重试状态,processed_at 不变)。
func (r *WebhookEventRepository) MarkProcessError(ctx context.Context, id string, processErr string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE webhook_events
		 SET process_error = $2, updated_at = NOW()
		 WHERE id = $1`, id, processErr)
	return err
}

func scanWebhookEvent(row pgx.Row) (*WebhookEvent, error) {
	var e WebhookEvent
	err := row.Scan(
		&e.ID, &e.ConnectorCode, &e.ExternalEventID, &e.SignatureValid, &e.PayloadJSON,
		&e.ReceivedAt, &e.ProcessedAt, &e.ProcessError,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWebhookNotFound
		}
		return nil, err
	}
	return &e, nil
}
