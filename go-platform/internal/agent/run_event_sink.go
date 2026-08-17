package agent

import (
	"context"
	"log"
)

// RunEventSink receives state-changing durable Run events after they have been
// durably applied, so the caller can advance dependent Workflow nodes. It is
// implemented outside this package to avoid an agent→workflow import cycle.
type RunEventSink interface {
	OnRunSucceeded(ctx context.Context, tenantID, runID, workflowInstanceID, nodeInstanceID string, outputJSON *string) error
	OnRunFailed(ctx context.Context, tenantID, runID, workflowInstanceID, nodeInstanceID string, errorJSON *string) error
	OnRunCancelled(ctx context.Context, tenantID, runID, workflowInstanceID, nodeInstanceID string) error
	OnRunInterrupted(ctx context.Context, tenantID, runID, workflowInstanceID, nodeInstanceID, interruptID, kind string) error
}

type runFinder interface {
	FindDurableRunByIDForTenant(ctx context.Context, tenantID, runID string) (*DurableRun, error)
}

// notifyRunEvent dispatches an applied terminal/interrupt event to the sink.
// It is best-effort: the event is already committed, so a sink failure must not
// fail the HTTP response. Lost node advancement is reconciled by the workflow
// convergence scanner (terminal run with a still-running node).
func (h *RuntimeHandler) notifyRunEvent(ctx context.Context, body RuntimeEventEnvelope) {
	if h.sink == nil || h.runs == nil {
		return
	}
	switch body.Type {
	case RuntimeEventRunSucceeded, RuntimeEventRunFailed, RuntimeEventRunCancelled, RuntimeEventRunInterrupted:
	default:
		return
	}
	run, err := h.runs.FindDurableRunByIDForTenant(ctx, body.TenantID, body.RunID)
	if err != nil {
		log.Printf("[runtime] notify: find run %s: %v", body.RunID, err)
		return
	}
	if run.WorkflowInstanceID == nil || run.NodeInstanceID == nil {
		return
	}
	workflowID, nodeID := *run.WorkflowInstanceID, *run.NodeInstanceID

	var notifyErr error
	switch body.Type {
	case RuntimeEventRunSucceeded:
		notifyErr = h.sink.OnRunSucceeded(ctx, body.TenantID, body.RunID, workflowID, nodeID, run.OutputSummaryJSON)
	case RuntimeEventRunFailed:
		notifyErr = h.sink.OnRunFailed(ctx, body.TenantID, body.RunID, workflowID, nodeID, run.ErrorJSON)
	case RuntimeEventRunCancelled:
		notifyErr = h.sink.OnRunCancelled(ctx, body.TenantID, body.RunID, workflowID, nodeID)
	case RuntimeEventRunInterrupted:
		interruptID, kind := interruptIdentity(body.Payload)
		notifyErr = h.sink.OnRunInterrupted(ctx, body.TenantID, body.RunID, workflowID, nodeID, interruptID, kind)
	}
	if notifyErr != nil {
		log.Printf("[runtime] notify workflow for run %s event %s failed: %v", body.RunID, body.Type, notifyErr)
	}
}

func interruptIdentity(payload map[string]any) (string, string) {
	if payload == nil {
		return "", ""
	}
	id, _ := payload["interrupt_id"].(string)
	kind, _ := payload["kind"].(string)
	if kind == "" {
		kind = "human"
	}
	return id, kind
}
