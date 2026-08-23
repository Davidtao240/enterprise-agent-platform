package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SidecarRegistration struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	ConnectorCode string   `json:"connector_code"`
	Version      string    `json:"version"`
	SidecarURL   string    `json:"sidecar_url"`
	AuthToken    string    `json:"-"`
	TimeoutMs    int       `json:"timeout_ms"`
	Capabilities []string  `json:"capabilities"`
	Status       string    `json:"status"`
	HealthStatus string    `json:"health_status"`
	LastError    string    `json:"last_error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type SidecarRepository struct {
	pool *pgxpool.Pool
}

func NewSidecarRepository(pool *pgxpool.Pool) *SidecarRepository {
	return &SidecarRepository{pool: pool}
}

func (r *SidecarRepository) Register(ctx context.Context, reg *SidecarRegistration) (*SidecarRegistration, error) {
	capsJSON, _ := json.Marshal(reg.Capabilities)
	err := r.pool.QueryRow(ctx,
		`INSERT INTO connector_sidecars
		 (tenant_id, connector_code, version, sidecar_url, auth_token, timeout_ms, capabilities_json, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,'active')
		 ON CONFLICT (tenant_id, connector_code, version) DO UPDATE SET
		   sidecar_url = EXCLUDED.sidecar_url,
		   auth_token = EXCLUDED.auth_token,
		   timeout_ms = EXCLUDED.timeout_ms,
		   capabilities_json = EXCLUDED.capabilities_json,
		   updated_at = NOW()
		 RETURNING id, created_at, updated_at`,
		reg.TenantID, reg.ConnectorCode, reg.Version, reg.SidecarURL,
		reg.AuthToken, reg.TimeoutMs, string(capsJSON),
	).Scan(&reg.ID, &reg.CreatedAt, &reg.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("register sidecar: %w", err)
	}
	return reg, nil
}

func (r *SidecarRepository) Get(ctx context.Context, tenantID, connectorCode, version string) (*SidecarRegistration, error) {
	var reg SidecarRegistration
	var capsJSON string
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, connector_code, version, sidecar_url, auth_token, timeout_ms,
		        capabilities_json::text, status, health_status, last_error, created_at, updated_at
		 FROM connector_sidecars
		 WHERE tenant_id = $1 AND connector_code = $2 AND version = $3`,
		tenantID, connectorCode, version,
	).Scan(&reg.ID, &reg.TenantID, &reg.ConnectorCode, &reg.Version,
		&reg.SidecarURL, &reg.AuthToken, &reg.TimeoutMs,
		&capsJSON, &reg.Status, &reg.HealthStatus, &reg.LastError, &reg.CreatedAt, &reg.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get sidecar: %w", err)
	}
	_ = json.Unmarshal([]byte(capsJSON), &reg.Capabilities)
	return &reg, nil
}

func (r *SidecarRepository) List(ctx context.Context, tenantID string) ([]SidecarRegistration, error) {
	var rows pgx.Rows
	var err error
	if tenantID != "" {
		rows, err = r.pool.Query(ctx,
			`SELECT id, tenant_id, connector_code, version, sidecar_url, auth_token, timeout_ms,
		        capabilities_json::text, status, health_status, last_error, created_at, updated_at
		 FROM connector_sidecars
		 WHERE tenant_id = $1
		 ORDER BY connector_code, version DESC`,
			tenantID)
	} else {
		rows, err = r.pool.Query(ctx,
			`SELECT id, tenant_id, connector_code, version, sidecar_url, auth_token, timeout_ms,
		        capabilities_json::text, status, health_status, last_error, created_at, updated_at
		 FROM connector_sidecars
		 ORDER BY connector_code, version DESC`)
	}
	if err != nil {
		return nil, fmt.Errorf("list sidecars: %w", err)
	}
	defer rows.Close()

	var items []SidecarRegistration
	for rows.Next() {
		var reg SidecarRegistration
		var capsJSON string
		if err := rows.Scan(&reg.ID, &reg.TenantID, &reg.ConnectorCode, &reg.Version,
			&reg.SidecarURL, &reg.AuthToken, &reg.TimeoutMs,
			&capsJSON, &reg.Status, &reg.HealthStatus, &reg.LastError, &reg.CreatedAt, &reg.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan sidecar: %w", err)
		}
		_ = json.Unmarshal([]byte(capsJSON), &reg.Capabilities)
		items = append(items, reg)
	}
	return items, nil
}

func (r *SidecarRepository) GetByID(ctx context.Context, tenantID, id string) (*SidecarRegistration, error) {
	var reg SidecarRegistration
	var capsJSON string
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, connector_code, version, sidecar_url, auth_token, timeout_ms,
		        capabilities_json::text, status, health_status, last_error, created_at, updated_at
		 FROM connector_sidecars
		 WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	).Scan(&reg.ID, &reg.TenantID, &reg.ConnectorCode, &reg.Version,
		&reg.SidecarURL, &reg.AuthToken, &reg.TimeoutMs,
		&capsJSON, &reg.Status, &reg.HealthStatus, &reg.LastError, &reg.CreatedAt, &reg.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("sidecar not found")
		}
		return nil, fmt.Errorf("get sidecar by id: %w", err)
	}
	_ = json.Unmarshal([]byte(capsJSON), &reg.Capabilities)
	return &reg, nil
}

func (r *SidecarRepository) UpdateHealth(ctx context.Context, tenantID, id, healthStatus, lastError string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE connector_sidecars
		 SET health_status = $3, last_error = $4, last_health_check = NOW(), updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2`,
		id, tenantID, healthStatus, lastError)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("sidecar not found")
	}
	return nil
}

func (r *SidecarRepository) UpdateStatus(ctx context.Context, tenantID, id, status string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE connector_sidecars SET status = $3, updated_at = NOW() WHERE id = $1 AND tenant_id = $2`,
		id, tenantID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("sidecar not found")
	}
	return nil
}

// SidecarConnector implements the Connector interface via HTTP sidecar
type SidecarConnector struct {
	manifest     ConnectorManifest
	capabilities []ConnectorCapability
	client       *SidecarClient
	mu           sync.RWMutex
}

func NewSidecarConnector(manifest ConnectorManifest, capabilities []ConnectorCapability, client *SidecarClient) *SidecarConnector {
	return &SidecarConnector{
		manifest:     manifest,
		capabilities: capabilities,
		client:       client,
	}
}

func (sc *SidecarConnector) Manifest() ConnectorManifest {
	return sc.manifest
}

func (sc *SidecarConnector) Capabilities() []ConnectorCapability {
	return sc.capabilities
}

func (sc *SidecarConnector) HealthCheck(ctx context.Context) ConnectorHealth {
	resp, err := sc.client.Health(ctx)
	if err != nil {
		return ConnectorHealth{
			Healthy:   false,
			Detail:    err.Error(),
			CheckedAt: time.Now(),
		}
	}
	healthy := resp != nil && resp.Healthy
	detail := ""
	if resp != nil {
		detail = resp.Detail
	}
	return ConnectorHealth{
		Healthy:   healthy,
		Detail:    detail,
		CheckedAt: time.Now(),
	}
}

func (sc *SidecarConnector) Execute(ctx context.Context, req *ConnectorRequest) (*ConnectorResult, error) {
	sidecarReq := &SidecarExecuteRequest{
		RequestID:  req.ToolCallID,
		TenantID:   req.TenantID,
		ToolCallID: req.ToolCallID,
		TraceID:    req.TraceID,
		Capability: req.Capability,
		Input:      req.Input,
	}

	resp, err := sc.client.Execute(ctx, sidecarReq)
	if err != nil {
		return &ConnectorResult{
			Status: ConnectorStatusIndeterminate,
			Error: &ConnectorError{
				Code:      "SIDECAR_NETWORK_ERROR",
				Message:   err.Error(),
				Retryable: true,
			},
		}, nil
	}

	status := ConnectorStatusSucceeded
	if resp.Status == "failed" {
		status = ConnectorStatusFailed
	}

	return &ConnectorResult{
		Status:            status,
		Output:            resp.Output,
		ExternalRequestID: resp.ExternalRequestID,
		ExternalObjectID:  resp.ExternalObjectID,
		Error:             resp.Error,
	}, nil
}

func (sc *SidecarConnector) Verify(ctx context.Context, req *ConnectorVerifyRequest) (*ConnectorVerifyResult, error) {
	sidecarReq := &SidecarVerifyRequest{
		RequestID:         fmt.Sprintf("verify-%s", req.ToolCallID),
		TenantID:          req.TenantID,
		ToolCallID:        req.ToolCallID,
		ExternalRequestID: req.ExternalRequestID,
		ExternalObjectID:  req.ExternalObjectID,
	}

	resp, err := sc.client.Verify(ctx, sidecarReq)
	if err != nil {
		return &ConnectorVerifyResult{
			Confirmed: false,
			Detail:    fmt.Sprintf("verify failed: %v", err),
		}, nil
	}

	return &ConnectorVerifyResult{
		Confirmed: resp.Confirmed,
		Detail:    resp.Detail,
		Observed:  resp.Observed,
	}, nil
}