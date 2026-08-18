package trace

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 封装 trace_events 表访问 (tenant_id 强制过滤)。
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository 创建 Repository。
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const insertSQL = `INSERT INTO trace_events
(trace_id, layer, parent_id, event_type, payload_json, timestamp, duration_ms, tenant_id, metadata_json)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

// InsertBatch 逐条写入一批事件 (调用方为异步 Recorder,量级可控)。
func (r *Repository) InsertBatch(ctx context.Context, events []*Event) error {
	for _, e := range events {
		if err := e.Validate(); err != nil {
			return err
		}
		var payloadJSON any
		if len(e.Payload) > 0 {
			payloadJSON = marshalJSON(e.Payload)
		}
		metadataJSON := "{}"
		if len(e.Metadata) > 0 {
			metadataJSON = marshalJSON(e.Metadata)
		}
		if _, err := r.pool.Exec(ctx, insertSQL,
			e.TraceID, e.Layer, e.ParentID, e.EventType, payloadJSON,
			e.Timestamp.UTC(), e.DurationMs, e.TenantID, metadataJSON,
		); err != nil {
			return err
		}
	}
	return nil
}

// ListByTraceID 按租户返回某 trace 的全部事件 (timestamp 升序)。
func (r *Repository) ListByTraceID(ctx context.Context, tenantID, traceID string) ([]*StoredEvent, error) {
	rows, err := r.pool.Query(ctx, eventSelect+` WHERE tenant_id = $1 AND trace_id = $2 ORDER BY timestamp ASC, id ASC`, tenantID, traceID)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// ListByTimeRange 按租户与时间窗口过滤事件 (Eval 数据源)。
func (r *Repository) ListByTimeRange(ctx context.Context, tenantID string, start, end time.Time, layer, eventType string) ([]*StoredEvent, error) {
	sql := eventSelect + ` WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp < $3`
	args := []any{tenantID, start, end}
	if layer != "" {
		args = append(args, layer)
		sql += ` AND layer = $` + strconv.Itoa(len(args))
	}
	if eventType != "" {
		args = append(args, eventType)
		sql += ` AND event_type = $` + strconv.Itoa(len(args))
	}
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

const eventSelect = `SELECT id, trace_id, layer, parent_id, event_type, payload_json, timestamp, duration_ms, tenant_id, metadata_json FROM trace_events`

func scanEvents(rows pgx.Rows) ([]*StoredEvent, error) {
	defer rows.Close()
	events := []*StoredEvent{}
	for rows.Next() {
		var e StoredEvent
		if err := rows.Scan(&e.ID, &e.TraceID, &e.Layer, &e.ParentID, &e.EventType, &e.PayloadJSON,
			&e.Timestamp, &e.DurationMs, &e.TenantID, &e.MetadataJSON); err != nil {
			return nil, err
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}

func marshalJSON(v map[string]any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
