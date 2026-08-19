package conversation

import (
	"context"
	"fmt"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/agent"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/google/uuid"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateConversation(ctx context.Context, conv *Conversation) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO conversations (tenant_id, created_by, thread_id, agent_package_code, title, status, budget_json)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 RETURNING id, created_at, updated_at`,
		conv.TenantID, conv.CreatedBy, conv.ThreadID, conv.AgentPackageCode, conv.Title, conv.Status, conv.BudgetJSON,
	).Scan(&conv.ID, &conv.CreatedAt, &conv.UpdatedAt)
}

func (r *Repository) GetConversation(ctx context.Context, id, tenantID string) (*Conversation, error) {
	conv := &Conversation{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, created_by, thread_id, agent_package_code, title, status, budget_json::text,
		        last_message_at, created_at, updated_at
		 FROM conversations WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`,
		id, tenantID,
	).Scan(&conv.ID, &conv.TenantID, &conv.CreatedBy, &conv.ThreadID, &conv.AgentPackageCode,
		&conv.Title, &conv.Status, &conv.BudgetJSON, &conv.LastMessageAt, &conv.CreatedAt, &conv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return conv, nil
}

func (r *Repository) GetConversationOwnedBy(ctx context.Context, id, tenantID, userID string) (*Conversation, error) {
	conv := &Conversation{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, created_by, thread_id, agent_package_code, title, status, budget_json::text,
		        last_message_at, created_at, updated_at
		 FROM conversations WHERE id = $1 AND tenant_id = $2 AND created_by = $3 AND deleted_at IS NULL`,
		id, tenantID, userID,
	).Scan(&conv.ID, &conv.TenantID, &conv.CreatedBy, &conv.ThreadID, &conv.AgentPackageCode,
		&conv.Title, &conv.Status, &conv.BudgetJSON, &conv.LastMessageAt, &conv.CreatedAt, &conv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return conv, nil
}

func (r *Repository) ListConversationsByUser(ctx context.Context, userID, tenantID, groupBy string) ([]ConversationGroup, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, created_by, thread_id, agent_package_code, title, status, budget_json::text,
		        last_message_at, created_at, updated_at
		 FROM conversations
		 WHERE tenant_id = $1 AND created_by = $2 AND deleted_at IS NULL AND status = 'active'
		 ORDER BY last_message_at NULLS LAST, created_at DESC`,
		tenantID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	var convs []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.TenantID, &c.CreatedBy, &c.ThreadID, &c.AgentPackageCode,
			&c.Title, &c.Status, &c.BudgetJSON, &c.LastMessageAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		convs = append(convs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if groupBy == "agent_package" {
		return groupByPackage(convs), nil
	}

	groups := []ConversationGroup{{
		AgentPackageCode: "",
		Conversations:    convs,
	}}
	return groups, nil
}

func groupByPackage(convs []Conversation) []ConversationGroup {
	seen := make(map[string]int)
	var groups []ConversationGroup
	for _, c := range convs {
		idx, ok := seen[c.AgentPackageCode]
		if !ok {
			idx = len(groups)
			seen[c.AgentPackageCode] = idx
			groups = append(groups, ConversationGroup{AgentPackageCode: c.AgentPackageCode})
		}
		groups[idx].Conversations = append(groups[idx].Conversations, c)
	}
	return groups
}

func (r *Repository) UpdateConversation(ctx context.Context, id, tenantID string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	var args []any
	query := "UPDATE conversations SET updated_at = now()"

	if title, ok := updates["title"]; ok {
		query += fmt.Sprintf(", title = $%d", len(args)+1)
		args = append(args, title)
	}
	if status, ok := updates["status"]; ok {
		query += fmt.Sprintf(", status = $%d", len(args)+1)
		args = append(args, status)
	}

	idPos := len(args) + 1
	tenantPos := len(args) + 2
	query += fmt.Sprintf(" WHERE id = $%d AND tenant_id = $%d AND deleted_at IS NULL", idPos, tenantPos)
	args = append(args, id, tenantID)

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("conversation %s not found", id)
	}
	return nil
}

func (r *Repository) SaveMessage(ctx context.Context, msg *ConversationMessage) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO conversation_messages (conversation_id, run_id, role, content, seq, tokens)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id, created_at`,
		msg.ConversationID, msg.RunID, msg.Role, msg.Content, msg.Seq, msg.Tokens,
	).Scan(&msg.ID, &msg.CreatedAt)
}

func (r *Repository) ListMessages(ctx context.Context, conversationID string, limit int, beforeMessageID string) ([]ConversationMessage, error) {
	query := `SELECT id, conversation_id, run_id, role, content, seq, tokens, created_at
	          FROM conversation_messages WHERE conversation_id = $1`
	args := []any{conversationID}
	argIdx := 2

	if beforeMessageID != "" {
		query += fmt.Sprintf(" AND seq < (SELECT seq FROM conversation_messages WHERE id = $%d)", argIdx)
		args = append(args, beforeMessageID)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY seq DESC LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var msgs []ConversationMessage
	for rows.Next() {
		var m ConversationMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.RunID, &m.Role,
			&m.Content, &m.Seq, &m.Tokens, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (r *Repository) GetActiveRunForConversation(ctx context.Context, conversationID, tenantID string) (*agent.DurableRun, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT dr.id, dr.thread_id, dr.tenant_id, dr.trace_id, dr.workflow_instance_id, dr.node_instance_id,
		        dr.parent_run_id, dr.graph_key, dr.graph_version, dr.configuration_snapshot_json::text,
		        dr.status, dr.attempt, dr.checkpoint_version, dr.lease_owner, dr.lease_expires_at, dr.heartbeat_at,
		        dr.budget_json::text, dr.output_summary_json::text, dr.usage_json::text, dr.error_json::text,
		        dr.started_at, dr.finished_at, dr.created_at, dr.updated_at, dr.metadata_json::text
		 FROM agent_runs dr
		 JOIN conversations c ON c.thread_id = dr.thread_id
		 WHERE c.id = $1 AND c.tenant_id = $2
		   AND dr.status IN ('queued','running','waiting_human','waiting_external')
		 ORDER BY dr.created_at DESC
		 LIMIT 1`,
		conversationID, tenantID,
	)

	run := &agent.DurableRun{}
	err := row.Scan(
		&run.ID, &run.ThreadID, &run.TenantID, &run.TraceID, &run.WorkflowInstanceID, &run.NodeInstanceID,
		&run.ParentRunID, &run.GraphKey, &run.GraphVersion, &run.ConfigurationSnapshotJSON,
		&run.Status, &run.Attempt, &run.CheckpointVersion, &run.LeaseOwner, &run.LeaseExpiresAt, &run.HeartbeatAt,
		&run.BudgetJSON, &run.OutputSummaryJSON, &run.UsageJSON, &run.ErrorJSON,
		&run.StartedAt, &run.FinishedAt, &run.CreatedAt, &run.UpdatedAt, &run.MetadataJSON,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get active run: %w", err)
	}
	return run, nil
}

func (r *Repository) IncrementSeq(ctx context.Context, conversationID string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var nextSeq int
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM conversation_messages WHERE conversation_id = $1 FOR UPDATE`,
		conversationID,
	).Scan(&nextSeq)
	if err != nil {
		return 0, fmt.Errorf("increment seq: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit seq: %w", err)
	}
	return nextSeq, nil
}

func (r *Repository) TouchLastMessageAt(ctx context.Context, conversationID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE conversations SET last_message_at = now(), updated_at = now() WHERE id = $1`,
		conversationID,
	)
	return err
}

func (r *Repository) InsertAgentThread(ctx context.Context, tenantID, createdBy, businessAppCode, title string) (string, error) {
	id := uuid.New().String()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO agent_threads (id, tenant_id, created_by, business_app_code, title, status)
		 VALUES ($1,$2,$3,$4,$5,'active')`,
		id, tenantID, createdBy, businessAppCode, title,
	)
	if err != nil {
		return "", fmt.Errorf("insert agent thread: %w", err)
	}
	return id, nil
}

func (r *Repository) CreateDurableRun(ctx context.Context, run *agent.DurableRun) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO agent_runs
		 (id, thread_id, tenant_id, trace_id, workflow_instance_id, node_instance_id,
		  graph_key, graph_version, configuration_snapshot_json, status, attempt, budget_json)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'queued',$10,$11)`,
		run.ID, run.ThreadID, run.TenantID, run.TraceID, run.WorkflowInstanceID, run.NodeInstanceID,
		run.GraphKey, run.GraphVersion, run.ConfigurationSnapshotJSON, run.Attempt, run.BudgetJSON,
	)
	if err != nil {
		return fmt.Errorf("create durable run: %w", err)
	}
	run.CreatedAt = now
	run.UpdatedAt = now
	run.Status = "queued"
	return nil
}