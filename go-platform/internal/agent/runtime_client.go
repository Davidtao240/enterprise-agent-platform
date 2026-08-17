package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type RuntimeV2Client struct {
	baseURL      string
	serviceToken string
	httpClient   *http.Client
}

type RuntimeV2GraphIdentity struct {
	Key     string `json:"key"`
	Version string `json:"version"`
}

type RuntimeV2Configuration struct {
	AgentDefinitionVersion string `json:"agent_definition_version"`
	ProfileOrSkillVersion  string `json:"profile_or_skill_version"`
	ModelConfigVersion     string `json:"model_config_version"`
}

type RuntimeV2TrustedContext struct {
	UserID       string `json:"user_id"`
	TenantID     string `json:"tenant_id"`
	DepartmentID string `json:"department_id,omitempty"`
}

type RuntimeV2Budget struct {
	DeadlineAt *time.Time `json:"deadline_at,omitempty"`
	MaxSteps   int        `json:"max_steps"`
	MaxCost    float64    `json:"max_cost"`
}

type RuntimeV2StartRequest struct {
	ProtocolVersion    string                  `json:"protocol_version"`
	RunID              string                  `json:"run_id"`
	ThreadID           string                  `json:"thread_id"`
	TraceID            string                  `json:"trace_id"`
	WorkflowInstanceID string                  `json:"workflow_instance_id,omitempty"`
	NodeInstanceID     string                  `json:"node_instance_id,omitempty"`
	BusinessAppCode    string                  `json:"business_app_code"`
	Graph              RuntimeV2GraphIdentity  `json:"graph"`
	Configuration      RuntimeV2Configuration  `json:"configuration"`
	Input              map[string]any          `json:"input"`
	TrustedContext     RuntimeV2TrustedContext `json:"trusted_context"`
	Budget             RuntimeV2Budget         `json:"budget"`
	Attempt            int                     `json:"attempt"`
	IdempotencyKey     string                  `json:"idempotency_key,omitempty"`
}

type RuntimeV2ResumeRequest struct {
	ProtocolVersion           string         `json:"protocol_version"`
	RunID                     string         `json:"run_id"`
	InterruptID               string         `json:"interrupt_id"`
	ExpectedCheckpointVersion int64          `json:"expected_checkpoint_version"`
	IdempotencyKey            string         `json:"idempotency_key"`
	ResumeInput               map[string]any `json:"resume_input"`
}

type RuntimeV2CancelRequest struct {
	ProtocolVersion string `json:"protocol_version"`
	RunID           string `json:"run_id"`
	Reason          string `json:"reason"`
	RequestedBy     string `json:"requested_by"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type RuntimeV2AcceptedResponse struct {
	ProtocolVersion   string    `json:"protocol_version"`
	RunID             string    `json:"run_id"`
	Status            string    `json:"status"`
	AcceptedAt        time.Time `json:"accepted_at"`
	CheckpointVersion int64     `json:"checkpoint_version"`
	Replayed          bool      `json:"replayed"`
}

func NewRuntimeV2Client(baseURL, serviceToken string) *RuntimeV2Client {
	return &RuntimeV2Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		serviceToken: serviceToken,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *RuntimeV2Client) Start(ctx context.Context, request *RuntimeV2StartRequest) (*RuntimeV2AcceptedResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("Runtime V2 Start request is required")
	}
	return c.post(ctx, "/internal/v2/agent-runs", request)
}

func (c *RuntimeV2Client) Resume(ctx context.Context, request *RuntimeV2ResumeRequest) (*RuntimeV2AcceptedResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("Runtime V2 Resume request is required")
	}
	return c.post(ctx, "/internal/v2/agent-runs/"+url.PathEscape(request.RunID)+"/resume", request)
}

func (c *RuntimeV2Client) Cancel(ctx context.Context, request *RuntimeV2CancelRequest) (*RuntimeV2AcceptedResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("Runtime V2 Cancel request is required")
	}
	return c.post(ctx, "/internal/v2/agent-runs/"+url.PathEscape(request.RunID)+"/cancel", request)
}

func (c *RuntimeV2Client) post(ctx context.Context, path string, body any) (*RuntimeV2AcceptedResponse, error) {
	if c.serviceToken == "" {
		return nil, fmt.Errorf("Runtime V2 service authentication is not configured")
	}
	requestJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal Runtime V2 request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(requestJSON))
	if err != nil {
		return nil, fmt.Errorf("create Runtime V2 request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Internal-Service-Token", c.serviceToken)

	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("call Runtime V2: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return nil, fmt.Errorf("Runtime V2 returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	accepted := &RuntimeV2AcceptedResponse{}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(accepted); err != nil {
		return nil, fmt.Errorf("decode Runtime V2 response: %w", err)
	}
	if accepted.ProtocolVersion != "2.0" || accepted.RunID == "" {
		return nil, fmt.Errorf("invalid Runtime V2 accepted response")
	}
	return accepted, nil
}
