package tool

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrToolCallNotFound 工具调用不存在。
var ErrToolCallNotFound = errors.New("tool call not found")

// ErrIdempotencyKeyConflict 幂等键冲突:同一 key 已有成功记录。
var ErrIdempotencyKeyConflict = errors.New("idempotency key conflict")

// ErrToolVersionNotFound 指定版本的工具不存在。
var ErrToolVersionNotFound = errors.New("tool version not found")

// ToolCallRepository 封装 tool_calls 表的 CRUD。
type ToolCallRepository struct {
	pool *pgxpool.Pool
}

// NewToolCallRepository 创建 ToolCallRepository。
func NewToolCallRepository(pool *pgxpool.Pool) *ToolCallRepository {
	return &ToolCallRepository{pool: pool}
}

// Create 创建工具调用记录,返回新建的 ToolCall。
// 幂等键冲突时返回 ErrIdempotencyKeyConflict。
func (r *ToolCallRepository) Create(ctx context.Context, req *ToolCallRequest) (*ToolCall, error) {
	id := uuid.NewString()
	now := time.Now().UTC()

	// 空 UUID 字段传 nil 而非空字符串,避免 PG 类型校验失败
	var stepID any
	if req.StepID != "" {
		stepID = req.StepID
	}
	var connectorBindingID any
	if req.ConnectorBindingID != "" {
		connectorBindingID = req.ConnectorBindingID
	}
	var externalRequestID any
	if req.ExternalRequestID != "" {
		externalRequestID = req.ExternalRequestID
	}

	var traceID any
	if req.TraceID != "" {
		traceID = req.TraceID
	}

	var timeoutAt any
	if req.TimeoutAt != nil {
		timeoutAt = *req.TimeoutAt
	}

	row := r.pool.QueryRow(ctx,
		`INSERT INTO tool_calls
			 (id, tenant_id, run_id, step_id, tool_id, tool_version, business_app_code,
			  connector_binding_id, policy_version, risk_level, status,
			  idempotency_key, input_hash, input_summary_json,
			  external_request_id, trace_id, timeout_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		 RETURNING id, tenant_id, run_id, step_id, tool_id, tool_version, business_app_code,
		   connector_binding_id, policy_version, risk_level, status,
		   idempotency_key, input_hash, input_summary_json, approval_task_id,
		   external_request_id, external_object_id, verification_json,
		   output_summary_json, error_json, timeout_at, retry_count, trace_id, is_dead_letter, created_at, updated_at`,
		id, req.TenantID, req.RunID, stepID, req.ToolID, req.ToolVersion, req.BusinessAppCode,
		connectorBindingID, req.PolicyVersion, req.RiskLevel, req.Status,
		req.IdempotencyKey, req.InputHash, req.InputSummaryJSON,
		externalRequestID, traceID, timeoutAt, now, now,
	)
	tc, err := scanToolCall(row)
	if err != nil {
		return nil, err
	}
	return tc, nil
}

// GetByID 按 ID 查找工具调用。
func (r *ToolCallRepository) GetByID(ctx context.Context, id string) (*ToolCall, error) {
	row := r.pool.QueryRow(ctx, toolCallSelect+" WHERE id = $1", id)
	return scanToolCall(row)
}

// GetByIdempotencyKey 按 (tenant, idempotency_key) 查找已存在的工具调用(幂等去重)。
func (r *ToolCallRepository) GetByIdempotencyKey(ctx context.Context, tenantID, idempotencyKey string) (*ToolCall, error) {
	query := toolCallSelect + " WHERE tenant_id = $1 AND idempotency_key = $2"
	row := r.pool.QueryRow(ctx, query, tenantID, idempotencyKey)
	tc, err := scanToolCall(row)
	if err != nil {
		if errors.Is(err, ErrToolCallNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return tc, nil
}

// UpdateStatus 更新工具调用状态及关联字段。
func (r *ToolCallRepository) UpdateStatus(ctx context.Context, id string, status ToolCallStatus, fields map[string]any) error {
	fields["status"] = status
	fields["updated_at"] = time.Now().UTC()
	idx := 1
	args := []any{}
	clauses := ""
	for k, v := range fields {
		clauses += fmt.Sprintf("%s = $%d, ", k, idx)
		args = append(args, v)
		idx++
	}
	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE tool_calls SET %s WHERE id = $%d`,
		clauses[:len(clauses)-2], idx,
	)
	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrToolCallNotFound
	}
	return nil
}

// ListByRun 按 run_id 列出工具调用。
func (r *ToolCallRepository) ListByRun(ctx context.Context, runID string) ([]*ToolCall, error) {
	rows, err := r.pool.Query(ctx, toolCallSelect+" WHERE run_id = $1 ORDER BY created_at", runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*ToolCall
	for rows.Next() {
		tc, err := scanToolCall(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, tc)
	}
	return result, nil
}

// ListTimedOutExecuting 列出 executing 且 timeout_at 已过的调用(M2-C 超时扫描器)。
// 排除死信记录(is_dead_letter=true):死信已进入终止态,不应被超时扫描器误处理。
func (r *ToolCallRepository) ListTimedOutExecuting(ctx context.Context, now time.Time, limit int) ([]*ToolCall, error) {
	rows, err := r.pool.Query(ctx,
		toolCallSelect+" WHERE status = 'executing' AND timeout_at IS NOT NULL AND timeout_at < $1 AND is_dead_letter = false ORDER BY timeout_at LIMIT $2",
		now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*ToolCall
	for rows.Next() {
		tc, err := scanToolCall(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, tc)
	}
	return result, nil
}

// ListByTrace 按 trace_id 列出工具调用(M2-E 全链 Trace 查询)。
func (r *ToolCallRepository) ListByTrace(ctx context.Context, traceID string) ([]*ToolCall, error) {
	rows, err := r.pool.Query(ctx,
		toolCallSelect+" WHERE trace_id = $1 ORDER BY created_at", traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*ToolCall
	for rows.Next() {
		tc, err := scanToolCall(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, tc)
	}
	return result, nil
}

// ListDeadLetters 列出死信调用(M2-C DLQ)。
func (r *ToolCallRepository) ListDeadLetters(ctx context.Context, limit int) ([]*ToolCall, error) {
	rows, err := r.pool.Query(ctx,
		toolCallSelect+" WHERE is_dead_letter ORDER BY updated_at DESC LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*ToolCall
	for rows.Next() {
		tc, err := scanToolCall(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, tc)
	}
	return result, nil
}

const toolCallSelect = `SELECT id, tenant_id, run_id, step_id, tool_id, tool_version, business_app_code,
	connector_binding_id, policy_version, risk_level, status,
	idempotency_key, input_hash, input_summary_json, approval_task_id,
	external_request_id, external_object_id, verification_json,
	output_summary_json, error_json, timeout_at, retry_count, trace_id, is_dead_letter, created_at, updated_at
FROM tool_calls`

func scanToolCall(row pgx.Row) (*ToolCall, error) {
	var tc ToolCall
	var stepID, inputSummary, approvalTaskID, externalReqID, externalObjID, verification, outputSummary, errJSON *string
	var traceID *string
	var createdAt, updatedAt time.Time
	err := row.Scan(
		&tc.ID, &tc.TenantID, &tc.RunID, &stepID, &tc.ToolID, &tc.ToolVersion, &tc.BusinessAppCode,
		&tc.ConnectorBindingID, &tc.PolicyVersion, &tc.RiskLevel, &tc.Status,
		&tc.IdempotencyKey, &tc.InputHash, &inputSummary, &approvalTaskID,
		&externalReqID, &externalObjID, &verification, &outputSummary, &errJSON,
		&tc.TimeoutAt, &tc.RetryCount, &traceID, &tc.IsDeadLetter, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrToolCallNotFound
		}
		return nil, err
	}
	tc.StepID = stepID
	tc.InputSummaryJSON = inputSummary
	tc.ApprovalTaskID = approvalTaskID
	tc.ExternalRequestID = externalReqID
	tc.ExternalObjectID = externalObjID
	tc.VerificationJSON = verification
	tc.OutputSummaryJSON = outputSummary
	tc.ErrorJSON = errJSON
	if traceID != nil {
		tc.TraceID = *traceID
	}
	tc.CreatedAt = createdAt
	tc.UpdatedAt = updatedAt
	return &tc, nil
}

// UpdateStatusGuarded 守卫式状态更新:仅当当前状态等于 expectedFrom 时迁移到 status。
// 并发竞争或状态已被其他路径推进时返回 ErrInvalidTransition,
// 记录不存在时返回 ErrToolCallNotFound。
func (r *ToolCallRepository) UpdateStatusGuarded(ctx context.Context, id string, expectedFrom, status ToolCallStatus, fields map[string]any) error {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["status"] = status
	fields["updated_at"] = time.Now().UTC()
	idx := 1
	args := []any{}
	clauses := ""
	for k, v := range fields {
		clauses += fmt.Sprintf("%s = $%d, ", k, idx)
		args = append(args, v)
		idx++
	}
	args = append(args, id, expectedFrom)
	query := fmt.Sprintf(
		`UPDATE tool_calls SET %s WHERE id = $%d AND status = $%d`,
		clauses[:len(clauses)-2], idx, idx+1,
	)
	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// 区分:记录不存在 vs 状态不匹配
		var current string
		if err := r.pool.QueryRow(ctx, `SELECT status FROM tool_calls WHERE id = $1`, id).Scan(&current); err != nil {
			return ErrToolCallNotFound
		}
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, current, status)
	}
	return nil
}
