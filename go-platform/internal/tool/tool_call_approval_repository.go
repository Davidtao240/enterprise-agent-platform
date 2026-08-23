package tool

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ToolCallApproval 高风险 Tool Call 的审批任务(M2-C)。
// PayloadHash 绑定 tool_calls.input_hash,保证审批内容与执行内容一致。
type ToolCallApproval struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	ToolCallID      string     `json:"tool_call_id"`
	BusinessAppCode string     `json:"business_app_code"`
	Title           string     `json:"title"`
	Status          string     `json:"status"` // pending / approved / rejected
	PayloadHash     string     `json:"payload_hash"`
	DecisionBy      *string    `json:"decision_by,omitempty"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ApprovalRepository 管理 Tool Call 审批任务的持久化。
// 直接写 approval_tasks 表(平台自有表,不引入跨包依赖);
// workflow 审批仍由 agent 包负责,两条路径互不干扰。
type ApprovalRepository struct {
	pool *pgxpool.Pool
}

// NewApprovalRepository 创建 ApprovalRepository。
func NewApprovalRepository(pool *pgxpool.Pool) *ApprovalRepository {
	return &ApprovalRepository{pool: pool}
}

// ErrApprovalNotFound 审批任务不存在。
var ErrApprovalNotFound = errors.New("approval task not found")

// CreateToolCallApproval 为高风险 Tool Call 创建审批任务(绑定不可变 PayloadHash)。
// 幂等:同一 tool_call_id 已有待决审批时直接返回已有任务。
func (r *ApprovalRepository) CreateToolCallApproval(ctx context.Context, approval *ToolCallApproval) error {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO approval_tasks
		 (tool_call_id, tenant_id, business_app_code, title, status, payload_hash)
		 VALUES ($1,$2,$3,$4,'pending',$5)
		 ON CONFLICT DO NOTHING
		 RETURNING id, tenant_id, created_at, updated_at`,
		approval.ToolCallID, approval.TenantID, approval.BusinessAppCode, approval.Title, approval.PayloadHash,
	).Scan(&approval.ID, &approval.TenantID, &approval.CreatedAt, &approval.UpdatedAt)
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHING 命中:返回已有任务
		return r.loadByToolCallID(ctx, approval)
	}
	return err
}

// FindToolCallApproval 按 tool_call_id 查找审批任务。
func (r *ApprovalRepository) FindToolCallApproval(ctx context.Context, toolCallID string) (*ToolCallApproval, error) {
	approval := &ToolCallApproval{}
	if err := r.scanRow(r.pool.QueryRow(ctx, toolCallApprovalSelect+" WHERE tool_call_id = $1", toolCallID), approval); err != nil {
		return nil, err
	}
	return approval, nil
}

// UpdateDecision 记录审批决定(approved/rejected)。
// 仅 pending 状态可决定;幂等:已是目标状态时直接成功。
func (r *ApprovalRepository) UpdateDecision(ctx context.Context, id, decision, decisionBy string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE approval_tasks
		 SET status = $2, decision_by = $3, decided_at = now(), updated_at = now()
		 WHERE id = $1 AND status = 'pending'`,
		id, decision, decisionBy,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// 幂等检查:已经决定过则成功
		var status string
		if err := r.pool.QueryRow(ctx, `SELECT status FROM approval_tasks WHERE id = $1`, id).Scan(&status); err != nil {
			return ErrApprovalNotFound
		}
		if status == decision {
			return nil
		}
		return errors.New("approval task already decided: " + status)
	}
	return nil
}

const toolCallApprovalSelect = `SELECT id, tenant_id, tool_call_id, business_app_code, title, status,
	payload_hash, decision_by, decided_at, created_at, updated_at
FROM approval_tasks`

func (r *ApprovalRepository) scanRow(row pgx.Row, approval *ToolCallApproval) error {
	var decisionBy *string
	err := row.Scan(
		&approval.ID, &approval.TenantID, &approval.ToolCallID, &approval.BusinessAppCode, &approval.Title,
		&approval.Status, &approval.PayloadHash, &decisionBy, &approval.DecidedAt,
		&approval.CreatedAt, &approval.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrApprovalNotFound
		}
		return err
	}
	approval.DecisionBy = decisionBy
	return nil
}

func (r *ApprovalRepository) loadByToolCallID(ctx context.Context, approval *ToolCallApproval) error {
	return r.scanRow(r.pool.QueryRow(ctx,
		toolCallApprovalSelect+" WHERE tool_call_id = $1", approval.ToolCallID), approval)
}
