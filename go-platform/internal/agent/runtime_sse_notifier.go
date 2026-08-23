package agent

import (
	"context"
	"encoding/json"
	"log"

	"github.com/enterprise-agent-platform/go-platform/internal/conversation"
)

// ConversationFinder resolves a durable Run's thread to the list of
// conversation ids that should receive its live runtime events.
type ConversationFinder interface {
	ListConversationIDsByThread(ctx context.Context, tenantID, threadID string) ([]string, error)
}

// ConversationPusher forwards an SSE event to all connected subscribers
// of a conversation id. Implemented by conversation.SSEWriter.
type ConversationPusher interface {
	PushEvent(conversationID string, event conversation.SSEEvent)
}

// RunToThreadResolver resolves a run_id -> thread_id lookup (agent_runs).
type RunToThreadResolver interface {
	FindDurableRunByIDForTenant(ctx context.Context, tenantID, runID string) (*DurableRun, error)
}

// RunTerminalHandler consumes terminal/interrupt runtime events for the
// conversation layer: assistant message persistence, message.completed and
// terminal SSE emission. Implemented by conversation.Service.
type RunTerminalHandler interface {
	HandleRunTerminalEvent(ctx context.Context, tenantID, threadID, runID, eventType string, payload map[string]any)
}

// ConversationSSENotifier broadcasts applied runtime events to the
// conversation SSE channel so the frontend can render step/tool events
// live without polling.
//
// Design notes:
//   - Best-effort: failures are logged, never returned. The event is already
//     durably applied; conversation SSE loss is recoverable via polling.
//   - Non-terminal runtime events (step.* / checkpoint.saved) are broadcast
//     as runtime.event.
//   - Terminal/interrupt events (run.succeeded / run.failed / run.cancelled /
//     run.interrupted / run.resumed) are forwarded to the RunTerminalHandler
//     which persists the assistant message and emits message.completed /
//     terminal SSE events.
type ConversationSSENotifier struct {
	find     RunToThreadResolver
	lookup   ConversationFinder
	pusher   ConversationPusher
	terminal RunTerminalHandler
}

func NewConversationSSENotifier(
	find RunToThreadResolver,
	lookup ConversationFinder,
	pusher ConversationPusher,
) *ConversationSSENotifier {
	return &ConversationSSENotifier{find: find, lookup: lookup, pusher: pusher}
}

// SetTerminalHandler wires terminal run events (assistant message save +
// terminal SSE) into the conversation layer.
func (n *ConversationSSENotifier) SetTerminalHandler(handler RunTerminalHandler) {
	n.terminal = handler
}

func isLiveRuntimeEvent(eventType string) bool {
	switch eventType {
	case RuntimeEventStepStarted, RuntimeEventStepCompleted, RuntimeEventStepFailed,
		RuntimeEventCheckpointSaved,
		RuntimeEventToolCalled, RuntimeEventArtifactReady:
		return true
	default:
		return false
	}
}

func isTerminalRuntimeEvent(eventType string) bool {
	switch eventType {
	case RuntimeEventRunSucceeded, RuntimeEventRunFailed, RuntimeEventRunCancelled,
		RuntimeEventRunInterrupted, RuntimeEventRunResumed:
		return true
	default:
		return false
	}
}

// Notify sends an applied runtime event to every conversation that owns
// the underlying Run's thread. Terminal events are forwarded to the
// terminal handler (assistant message + terminal SSE); live events are
// broadcast as runtime.event.
func (n *ConversationSSENotifier) Notify(ctx context.Context, body RuntimeEventEnvelope) {
	if n == nil || n.find == nil || n.lookup == nil || n.pusher == nil {
		return
	}

	run, err := n.find.FindDurableRunByIDForTenant(ctx, body.TenantID, body.RunID)
	if err != nil {
		log.Printf("[runtime-sse] find run %s for event %s failed: %v", body.RunID, body.Type, err)
		return
	}
	if run == nil || run.ThreadID == "" {
		return
	}

	if isTerminalRuntimeEvent(body.Type) {
		if n.terminal != nil {
			n.terminal.HandleRunTerminalEvent(ctx, body.TenantID, run.ThreadID, body.RunID, body.Type, body.Payload)
		}
		return
	}
	if !isLiveRuntimeEvent(body.Type) {
		return
	}

	convIDs, err := n.lookup.ListConversationIDsByThread(ctx, body.TenantID, run.ThreadID)
	if err != nil {
		log.Printf("[runtime-sse] list conversations for thread %s failed: %v", run.ThreadID, err)
		return
	}
	if len(convIDs) == 0 {
		return
	}

	payload := map[string]any{
		"run_id":      body.RunID,
		"sequence":    body.Sequence,
		"attempt":     body.Attempt,
		"occurred_at": body.OccurredAt,
		"type":        body.Type,
	}
	if body.Payload != nil {
		payload["payload"] = body.Payload
	}
	if body.CheckpointVersion != nil {
		payload["checkpoint_version"] = *body.CheckpointVersion
	}

	evt := conversation.SSEEvent{
		ID:    body.EventID,
		Event: "runtime.event",
		Data:  payload,
	}
	for _, id := range convIDs {
		n.pusher.PushEvent(id, evt)
	}
}

// eventPayloadJSON is a small helper for future tool/artifact event adapters.
func eventPayloadJSON(payload map[string]any) string {
	b, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(b)
}
