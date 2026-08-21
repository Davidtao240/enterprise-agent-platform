package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type conversationStore interface {
	CreateConversation(ctx context.Context, conv *Conversation) error
	GetConversation(ctx context.Context, id, tenantID string) (*Conversation, error)
	GetConversationOwnedBy(ctx context.Context, id, tenantID, userID string) (*Conversation, error)
	ListConversationsByUser(ctx context.Context, userID, tenantID, groupBy string) ([]ConversationGroup, error)
	UpdateConversation(ctx context.Context, id, tenantID string, updates map[string]any) error
	SaveMessage(ctx context.Context, msg *ConversationMessage) error
	ListMessages(ctx context.Context, conversationID string, limit int, beforeMessageID string) ([]ConversationMessage, error)
	GetActiveRunForConversation(ctx context.Context, conversationID, tenantID string) (*DurableRunRef, error)
	IncrementSeq(ctx context.Context, conversationID string) (int, error)
	TouchLastMessageAt(ctx context.Context, conversationID string) error
	InsertAgentThread(ctx context.Context, tenantID, createdBy, businessAppCode, title string) (string, error)
	CreateDurableRun(ctx context.Context, run *DurableRunRef) error
}

type SSEEventPusher interface {
	PushEvent(conversationID string, event SSEEvent)
	GetEventsSince(conversationID string, sinceSeq int) []SSEEvent
	Subscribe(conversationID string) (<-chan SSEEvent, func())
}

type Service struct {
	store         conversationStore
	pusher        SSEEventPusher
	dispatchAgent DispatchAgentFunc
}

func NewService(store conversationStore, pusher SSEEventPusher, dispatchAgent DispatchAgentFunc) *Service {
	return &Service{store: store, pusher: pusher, dispatchAgent: dispatchAgent}
}

func (s *Service) CreateConversation(ctx context.Context, userID, tenantID string, req *CreateConversationRequest) (*Conversation, error) {
	title := req.Title
	if title == "" {
		title = fmt.Sprintf("Conversation %s", time.Now().Format("2006-01-02 15:04"))
	}

	threadID, err := s.store.InsertAgentThread(ctx, tenantID, userID, req.AgentPackageCode, title)
	if err != nil {
		return nil, fmt.Errorf("create agent thread: %w", err)
	}

	conv := &Conversation{
		TenantID:         tenantID,
		CreatedBy:        userID,
		ThreadID:         threadID,
		AgentPackageCode: req.AgentPackageCode,
		Title:            &title,
		Status:           string(ConvStatusActive),
	}
	if err := s.store.CreateConversation(ctx, conv); err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}

	return conv, nil
}

func (s *Service) ListConversations(ctx context.Context, userID, tenantID, groupBy string) ([]ConversationGroup, error) {
	return s.store.ListConversationsByUser(ctx, userID, tenantID, groupBy)
}

func (s *Service) GetConversation(ctx context.Context, id, tenantID string) (*Conversation, []ConversationMessage, error) {
	conv, err := s.store.GetConversation(ctx, id, tenantID)
	if err != nil {
		return nil, nil, fmt.Errorf("get conversation: %w", err)
	}

	msgs, err := s.store.ListMessages(ctx, id, 50, "")
	if err != nil {
		return nil, nil, fmt.Errorf("list messages: %w", err)
	}

	return conv, msgs, nil
}

func (s *Service) UpdateConversation(ctx context.Context, id, tenantID string, req *UpdateConversationRequest) error {
	updates := map[string]any{}
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	return s.store.UpdateConversation(ctx, id, tenantID, updates)
}

func (s *Service) SendMessage(ctx context.Context, userID, tenantID, conversationID string, req *SendMessageRequest) (*SendMessageResponse, error) {
	conv, err := s.store.GetConversationOwnedBy(ctx, conversationID, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, fmt.Errorf("conversation not found or not owned by user")
	}

	seq, err := s.store.IncrementSeq(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("increment seq: %w", err)
	}

	userMsg := &ConversationMessage{
		ConversationID: conversationID,
		Role:           string(RoleUser),
		Content:        req.Content,
		Seq:            seq,
	}
	if err := s.store.SaveMessage(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("save user message: %w", err)
	}

	if err := s.store.TouchLastMessageAt(ctx, conversationID); err != nil {
		return nil, fmt.Errorf("touch last_message_at: %w", err)
	}

	if s.dispatchAgent == nil {
		return nil, fmt.Errorf("agent dispatcher not configured")
	}

	runID, status, err := s.dispatchAgent(ctx, userID, tenantID, conv.AgentPackageCode, conv.ThreadID, req.Content)
	if err != nil {
		return nil, fmt.Errorf("dispatch agent: %w", err)
	}

	s.pusher.PushEvent(conversationID, SSEEvent{
		ID:    uuid.New().String(),
		Event: "run.started",
		Data: map[string]any{
			"run_id":     runID,
			"message_id": userMsg.ID,
			"seq":        seq,
		},
	})

	return &SendMessageResponse{
		MessageID: userMsg.ID,
		RunID:     runID,
		Status:    status,
	}, nil
}

func (s *Service) AnswerClarification(ctx context.Context, userID, tenantID, conversationID string, req *AnswerClarificationRequest) error {
	conv, err := s.store.GetConversationOwnedBy(ctx, conversationID, tenantID, userID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return fmt.Errorf("conversation not found or not owned by user")
	}

	run, err := s.store.GetActiveRunForConversation(ctx, conversationID, tenantID)
	if err != nil {
		return fmt.Errorf("get active run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("no active run for conversation %s", conversationID)
	}

	payloadBytes, _ := json.Marshal(req.Answers)
	s.pusher.PushEvent(conversationID, SSEEvent{
		ID:    uuid.New().String(),
		Event: "clarification.answered",
		Data: map[string]any{
			"run_id":  run.ID,
			"answers": string(payloadBytes),
		},
	})

	if err := s.store.UpdateConversation(ctx, conversationID, tenantID, map[string]any{
		"status": string(ConvStatusActive),
	}); err != nil {
		return fmt.Errorf("reopen conversation: %w", err)
	}

	return nil
}

func (s *Service) CancelRun(ctx context.Context, userID, tenantID, conversationID string) error {
	conv, err := s.store.GetConversationOwnedBy(ctx, conversationID, tenantID, userID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return fmt.Errorf("conversation not found or not owned by user")
	}

	run, err := s.store.GetActiveRunForConversation(ctx, conversationID, tenantID)
	if err != nil {
		return fmt.Errorf("get active run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("no active run to cancel for conversation %s", conversationID)
	}

	if err := s.store.UpdateConversation(ctx, conversationID, tenantID, map[string]any{
		"status": string(ConvStatusClosed),
	}); err != nil {
		return fmt.Errorf("close conversation: %w", err)
	}

	s.pusher.PushEvent(conversationID, SSEEvent{
		ID:    uuid.New().String(),
		Event: "run.cancelled",
		Data: map[string]any{
			"run_id": run.ID,
		},
	})

	return nil
}

func (s *Service) GetSSEEvents(ctx context.Context, conversationID, tenantID string) (<-chan SSEEvent, error) {
	_, err := s.store.GetConversation(ctx, conversationID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	ch, _ := s.pusher.Subscribe(conversationID)
	return ch, nil
}
