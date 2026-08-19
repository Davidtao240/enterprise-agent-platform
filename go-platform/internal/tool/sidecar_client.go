package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type SidecarClient struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

type SidecarExecuteRequest struct {
	RequestID  string              `json:"request_id"`
	TenantID   string              `json:"tenant_id"`
	ToolCallID string              `json:"tool_call_id"`
	TraceID    string              `json:"trace_id,omitempty"`
	Capability string              `json:"capability"`
	Input      map[string]any      `json:"input"`
}

type SidecarExecuteResponse struct {
	Status            string              `json:"status"`
	Output            map[string]any      `json:"output,omitempty"`
	ExternalRequestID string              `json:"external_request_id,omitempty"`
	ExternalObjectID  string              `json:"external_object_id,omitempty"`
	Error             *ConnectorError     `json:"error,omitempty"`
}

type SidecarVerifyRequest struct {
	RequestID         string `json:"request_id"`
	TenantID          string `json:"tenant_id"`
	ToolCallID        string `json:"tool_call_id"`
	ExternalRequestID string `json:"external_request_id,omitempty"`
	ExternalObjectID  string `json:"external_object_id,omitempty"`
}

type SidecarVerifyResponse struct {
	Confirmed bool             `json:"confirmed"`
	Detail    string           `json:"detail,omitempty"`
	Observed  map[string]any   `json:"observed,omitempty"`
}

type SidecarHealthResponse struct {
	Healthy   bool      `json:"healthy"`
	Detail    string    `json:"detail,omitempty"`
	Version   string    `json:"version,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type SidecarManifestResponse struct {
	ConnectorCode    string                    `json:"connector_code"`
	Version          string                    `json:"version"`
	ConnectorType    string                    `json:"connector_type"`
	Capabilities     []ConnectorCapability     `json:"capabilities"`
	AuthType         string                    `json:"auth_type"`
	ReleaseStage     string                    `json:"release_stage"`
}

func NewSidecarClient(baseURL, authToken string, timeoutMs int) *SidecarClient {
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}
	return &SidecarClient{
		baseURL:   baseURL,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutMs) * time.Millisecond,
		},
	}
}

func (c *SidecarClient) doRequest(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("sidecar request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func (c *SidecarClient) Execute(ctx context.Context, req *SidecarExecuteRequest) (*SidecarExecuteResponse, error) {
	body, statusCode, err := c.doRequest(ctx, http.MethodPost, "/execute", req)
	if err != nil {
		return nil, fmt.Errorf("sidecar execute: %w", err)
	}
	if statusCode >= 500 {
		return &SidecarExecuteResponse{
			Status: "failed",
			Error: &ConnectorError{
				Code:      "SIDECAR_UNAVAILABLE",
				Message:   fmt.Sprintf("sidecar returned %d", statusCode),
				Retryable: true,
			},
		}, nil
	}
	if statusCode >= 400 {
		var errResp SidecarExecuteResponse
		if err := json.Unmarshal(body, &errResp); err == nil {
			return &errResp, nil
		}
		return &SidecarExecuteResponse{
			Status: "failed",
			Error: &ConnectorError{
				Code:      "SIDECAR_BAD_REQUEST",
				Message:   fmt.Sprintf("sidecar returned %d", statusCode),
				Retryable: false,
			},
		}, nil
	}

	var resp SidecarExecuteResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode sidecar response: %w", err)
	}
	return &resp, nil
}

func (c *SidecarClient) Verify(ctx context.Context, req *SidecarVerifyRequest) (*SidecarVerifyResponse, error) {
	body, statusCode, err := c.doRequest(ctx, http.MethodPost, "/verify", req)
	if err != nil {
		return nil, fmt.Errorf("sidecar verify: %w", err)
	}
	if statusCode >= 400 {
		return &SidecarVerifyResponse{Confirmed: false, Detail: fmt.Sprintf("sidecar returned %d", statusCode)}, nil
	}

	var resp SidecarVerifyResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode verify response: %w", err)
	}
	return &resp, nil
}

func (c *SidecarClient) Health(ctx context.Context) (*SidecarHealthResponse, error) {
	body, statusCode, err := c.doRequest(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return nil, fmt.Errorf("sidecar health: %w", err)
	}
	if statusCode >= 400 {
		return &SidecarHealthResponse{Healthy: false, Detail: fmt.Sprintf("HTTP %d", statusCode)}, nil
	}

	var resp SidecarHealthResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode health response: %w", err)
	}
	return &resp, nil
}

func (c *SidecarClient) GetManifest(ctx context.Context) (*SidecarManifestResponse, error) {
	body, statusCode, err := c.doRequest(ctx, http.MethodGet, "/manifest", nil)
	if err != nil {
		return nil, fmt.Errorf("sidecar manifest: %w", err)
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("sidecar returned %d", statusCode)
	}

	var resp SidecarManifestResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	return &resp, nil
}