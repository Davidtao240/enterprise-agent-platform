package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
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
	ListConversationIDsByThread(ctx context.Context, tenantID, threadID string) ([]string, error)
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
	runCtrl       RunController
}

func NewService(store conversationStore, pusher SSEEventPusher, dispatchAgent DispatchAgentFunc) *Service {
	return &Service{store: store, pusher: pusher, dispatchAgent: dispatchAgent}
}

// SetRunController wires control-plane cancel/resume for conversation runs.
func (s *Service) SetRunController(ctrl RunController) {
	s.runCtrl = ctrl
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
	return s.store.ListConversationsByUser(ctx, userID, tenantID, normalizeGroupBy(groupBy))
}

// normalizeGroupBy accepts both "agent_package" and "agent_package_code".
func normalizeGroupBy(v string) string {
	if v == "agent_package" || v == "agent_package_code" {
		return "agent_package"
	}
	return ""
}

func (s *Service) GetConversation(ctx context.Context, userID, id, tenantID string) (*Conversation, []ConversationMessage, error) {
	conv, err := s.store.GetConversationOwnedBy(ctx, id, tenantID, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("get conversation: %w", err)
	}

	msgs, err := s.store.ListMessages(ctx, id, 50, "")
	if err != nil {
		return nil, nil, fmt.Errorf("list messages: %w", err)
	}

	return conv, msgs, nil
}

func (s *Service) UpdateConversation(ctx context.Context, userID, id, tenantID string, req *UpdateConversationRequest) error {
	if _, err := s.store.GetConversationOwnedBy(ctx, id, tenantID, userID); err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}

	updates := map[string]any{}
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	return s.store.UpdateConversation(ctx, id, tenantID, updates)
}

// SendMessage persists the user message and dispatches an agent run.
//
// Ordering (M7 gate): the durable run is created first (Runtime V2 accepted,
// fast) and the user message is persisted only after dispatch succeeds, so a
// dispatch failure returns an error without leaving a dangling user message
// that a client retry would duplicate.
func (s *Service) SendMessage(ctx context.Context, userID, tenantID, conversationID string, req *SendMessageRequest) (*SendMessageResponse, error) {
	conv, err := s.store.GetConversationOwnedBy(ctx, conversationID, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	if s.dispatchAgent == nil {
		return nil, fmt.Errorf("agent dispatcher not configured")
	}

	runID, status, err := s.dispatchAgent(ctx, userID, tenantID, conv.AgentPackageCode, conv.ThreadID, req.Content)
	if err != nil {
		return nil, fmt.Errorf("dispatch agent: %w", err)
	}

	seq, err := s.store.IncrementSeq(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("increment seq: %w", err)
	}

	userMsg := &ConversationMessage{
		ConversationID: conversationID,
		RunID:          &runID,
		Role:           string(RoleUser),
		Content:        req.Content,
		Seq:            seq,
	}
	if err := s.store.SaveMessage(ctx, userMsg); err != nil {
		// Best-effort: cancel the just-dispatched run so a retry does not
		// leave an orphaned execution without a persisted user message.
		if s.runCtrl != nil {
			if cancelErr := s.runCtrl.CancelRun(ctx, tenantID, runID, "message persist failed", userID); cancelErr != nil {
				log.Printf("[conversation] cancel run %s after message persist failure: %v", runID, cancelErr)
			}
		}
		return nil, fmt.Errorf("save user message: %w", err)
	}

	if err := s.store.TouchLastMessageAt(ctx, conversationID); err != nil {
		return nil, fmt.Errorf("touch last_message_at: %w", err)
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

// AnswerClarification submits clarification answers and resumes the
// interrupted run via the runtime control plane.
func (s *Service) AnswerClarification(ctx context.Context, userID, tenantID, conversationID string, req *AnswerClarificationRequest) error {
	if _, err := s.store.GetConversationOwnedBy(ctx, conversationID, tenantID, userID); err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}

	run, err := s.store.GetActiveRunForConversation(ctx, conversationID, tenantID)
	if err != nil {
		return fmt.Errorf("get active run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("no active run for conversation %s", conversationID)
	}

	if s.runCtrl != nil {
		if err := s.runCtrl.ResumeRun(ctx, tenantID, run.ID, req.InterruptID, req.Answers); err != nil {
			return fmt.Errorf("resume run %s: %w", run.ID, err)
		}
	}

	payloadBytes, _ := json.Marshal(req.Answers)
	s.pusher.PushEvent(conversationID, SSEEvent{
		ID:    uuid.New().String(),
		Event: "clarification.answered",
		Data: map[string]any{
			"run_id":      run.ID,
			"interrupt_id": req.InterruptID,
			"answers":     string(payloadBytes),
		},
	})

	return nil
}

// CancelRun cancels the active run of the conversation via the runtime
// control plane. The conversation itself stays open for the next message.
func (s *Service) CancelRun(ctx context.Context, userID, tenantID, conversationID string) error {
	if _, err := s.store.GetConversationOwnedBy(ctx, conversationID, tenantID, userID); err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}

	run, err := s.store.GetActiveRunForConversation(ctx, conversationID, tenantID)
	if err != nil {
		return fmt.Errorf("get active run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("no active run to cancel for conversation %s", conversationID)
	}

	if s.runCtrl != nil {
		if err := s.runCtrl.CancelRun(ctx, tenantID, run.ID, "user requested cancel", userID); err != nil {
			return fmt.Errorf("cancel run %s: %w", run.ID, err)
		}
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

// HandleRunTerminalEvent consumes terminal/interrupt runtime events for a
// thread: on run.succeeded it persists the assistant message and emits
// message.completed + run.completed SSE; on run.failed / run.cancelled it
// emits the terminal SSE; on run.interrupted it emits clarification.requested.
// Best-effort: failures are logged, never propagated to the event ingest path.
func (s *Service) HandleRunTerminalEvent(ctx context.Context, tenantID, threadID, runID, eventType string, payload map[string]any) {
	convIDs, err := s.store.ListConversationIDsByThread(ctx, tenantID, threadID)
	if err != nil {
		log.Printf("[conversation] list conversations for thread %s failed: %v", threadID, err)
		return
	}
	if len(convIDs) == 0 {
		return
	}

	switch eventType {
	case "run.succeeded":
		content := assistantContentFromPayload(payload)
		for _, convID := range convIDs {
			msgID, seq, err := s.saveAssistantMessage(ctx, convID, runID, content)
			if err != nil {
				log.Printf("[conversation] save assistant message for conversation %s failed: %v", convID, err)
				continue
			}
			s.pusher.PushEvent(convID, SSEEvent{
				ID:    uuid.New().String(),
				Event: "message.completed",
				Data: map[string]any{
					"message_id": msgID,
					"run_id":     runID,
					"role":       "assistant",
					"content":    content,
					"seq":        seq,
				},
			})
			s.pusher.PushEvent(convID, SSEEvent{
				ID:    uuid.New().String(),
				Event: "run.completed",
				Data:  map[string]any{"run_id": runID},
			})
		}
	case "run.failed":
		errMsg := errorFromPayload(payload)
		for _, convID := range convIDs {
			s.pusher.PushEvent(convID, SSEEvent{
				ID:    uuid.New().String(),
				Event: "run.failed",
				Data: map[string]any{
					"run_id": runID,
					"error":  errMsg,
				},
			})
		}
	case "run.cancelled":
		for _, convID := range convIDs {
			s.pusher.PushEvent(convID, SSEEvent{
				ID:    uuid.New().String(),
				Event: "run.cancelled",
				Data:  map[string]any{"run_id": runID},
			})
		}
	case "run.interrupted":
		for _, convID := range convIDs {
			s.pusher.PushEvent(convID, SSEEvent{
				ID:    uuid.New().String(),
				Event: "clarification.requested",
				Data: map[string]any{
					"run_id":  runID,
					"payload": payload,
				},
			})
		}
	case "run.resumed":
		for _, convID := range convIDs {
			s.pusher.PushEvent(convID, SSEEvent{
				ID:    uuid.New().String(),
				Event: "run.started",
				Data:  map[string]any{"run_id": runID},
			})
		}
	}
}

func (s *Service) saveAssistantMessage(ctx context.Context, conversationID, runID, content string) (string, int, error) {
	seq, err := s.store.IncrementSeq(ctx, conversationID)
	if err != nil {
		return "", 0, fmt.Errorf("increment seq: %w", err)
	}

	msg := &ConversationMessage{
		ConversationID: conversationID,
		RunID:          &runID,
		Role:           string(RoleAssistant),
		Content:        content,
		Seq:            seq,
	}
	if err := s.store.SaveMessage(ctx, msg); err != nil {
		return "", 0, fmt.Errorf("save assistant message: %w", err)
	}

	if err := s.store.TouchLastMessageAt(ctx, conversationID); err != nil {
		log.Printf("[conversation] touch last_message_at for %s failed: %v", conversationID, err)
	}
	return msg.ID, seq, nil
}

// assistantContentFromPayload extracts the assistant reply text from a
// run.succeeded event payload ({output: {summary, ...}, usage: {...}}).
func assistantContentFromPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if output, ok := payload["output"].(map[string]any); ok {
		if summary, ok := output["summary"].(string); ok && strings.TrimSpace(summary) != "" {
			return summary
		}
		if b, err := json.Marshal(output); err == nil {
			return string(b)
		}
	}
	if b, err := json.Marshal(payload); err == nil {
		return string(b)
	}
	return ""
}

func errorFromPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	if errObj, ok := payload["error"].(map[string]any); ok {
		return errObj
	}
	return payload
}
