package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SidecarService struct {
	repo       *SidecarRepository
	runtime    *ConnectorRuntime
	mu         sync.RWMutex
	registrations map[string]*SidecarRegistration
}

func NewSidecarService(repo *SidecarRepository, runtime *ConnectorRuntime) *SidecarService {
	return &SidecarService{
		repo:          repo,
		runtime:       runtime,
		registrations: make(map[string]*SidecarRegistration),
	}
}

// validateSidecarURL mitigates SSRF: only http(s) URLs without embedded
// credentials are accepted, and the resolved host must not be link-local
// (covers the cloud metadata endpoint 169.254.169.254) or unspecified.
// Loopback/private ranges stay reachable because sidecars are deployed in
// the same network as the platform by design.
func validateSidecarURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid sidecar url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("sidecar url scheme must be http or https")
	}
	if u.User != nil {
		return fmt.Errorf("sidecar url must not embed credentials")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("sidecar url host is required")
	}

	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		resolved, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("resolve sidecar host %s: %w", host, err)
		}
		ips = resolved
	}

	for _, ip := range ips {
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("sidecar url host %s resolves to a link-local/unspecified address", host)
		}
	}
	return nil
}

func (s *SidecarService) RegisterSidecar(ctx context.Context, reg *SidecarRegistration) (*SidecarRegistration, error) {
	reg.ID = uuid.NewString()

	if err := validateSidecarURL(reg.SidecarURL); err != nil {
		return nil, err
	}

	existing, err := s.repo.Get(ctx, reg.TenantID, reg.ConnectorCode, reg.Version)
	if err != nil {
		return nil, fmt.Errorf("get existing: %w", err)
	}

	var result *SidecarRegistration
	if existing != nil {
		reg.ID = existing.ID
		result, err = s.repo.Register(ctx, reg)
		if err != nil {
			return nil, fmt.Errorf("update sidecar: %w", err)
		}
	} else {
		result, err = s.repo.Register(ctx, reg)
		if err != nil {
			return nil, fmt.Errorf("register sidecar: %w", err)
		}
	}

	client := NewSidecarClient(result.SidecarURL, result.AuthToken, result.TimeoutMs)
	manifest := ConnectorManifest{
		ConnectorCode: result.ConnectorCode,
		Version:       result.Version,
		ConnectorType: "http_sidecar",
		AuthType:      "api_key",
		ReleaseStage:  "production",
	}

	var caps []ConnectorCapability
	for _, c := range result.Capabilities {
		caps = append(caps, ConnectorCapability{
			Name:        c,
			Kind:        CapabilityKindRead,
			InputSchema: map[string]any{"type": "object"},
		})
	}

	connector := NewSidecarConnector(manifest, caps, client)
	if err := s.runtime.RegisterConnector(ctx, connector); err != nil {
		return nil, fmt.Errorf("register connector: %w", err)
	}

	s.mu.Lock()
	s.registrations[result.ID] = result
	s.mu.Unlock()

	return result, nil
}

func (s *SidecarService) ListSidecars(ctx context.Context, tenantID string) ([]SidecarRegistration, error) {
	return s.repo.List(ctx, tenantID)
}

func (s *SidecarService) DeregisterSidecar(ctx context.Context, tenantID, id string) error {
	if _, err := s.GetSidecar(ctx, tenantID, id); err != nil {
		return err
	}

	if err := s.repo.UpdateStatus(ctx, tenantID, id, "inactive"); err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.registrations, id)
	s.mu.Unlock()
	return nil
}

func (s *SidecarService) HealthCheck(ctx context.Context, tenantID, id string) (*SidecarRegistration, error) {
	reg, err := s.GetSidecar(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	client := NewSidecarClient(reg.SidecarURL, reg.AuthToken, reg.TimeoutMs)
	resp, err := client.Health(ctx)
	if err != nil {
		_ = s.repo.UpdateHealth(ctx, tenantID, id, "unhealthy", err.Error())
		reg.HealthStatus = "unhealthy"
		reg.LastError = err.Error()
		reg.UpdatedAt = time.Now()
		return reg, nil
	}

	healthStatus := "unhealthy"
	if resp != nil && resp.Healthy {
		healthStatus = "healthy"
	}

	errMsg := ""
	if resp != nil {
		errMsg = resp.Detail
	}
	_ = s.repo.UpdateHealth(ctx, tenantID, id, healthStatus, errMsg)

	s.mu.Lock()
	if current, ok := s.registrations[id]; ok {
		current.HealthStatus = healthStatus
		current.LastError = errMsg
		current.UpdatedAt = time.Now()
	}
	s.mu.Unlock()

	reg.HealthStatus = healthStatus
	reg.LastError = errMsg
	return reg, nil
}

func (s *SidecarService) GetSidecar(ctx context.Context, tenantID, id string) (*SidecarRegistration, error) {
	s.mu.RLock()
	reg, ok := s.registrations[id]
	s.mu.RUnlock()
	if ok {
		if reg.TenantID != tenantID {
			return nil, fmt.Errorf("sidecar not found")
		}
		return reg, nil
	}

	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *SidecarService) LoadExistingSidecars(ctx context.Context, tenantID string) error {
	regs, err := s.repo.List(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("list sidecars: %w", err)
	}

	for _, reg := range regs {
		if reg.Status != "active" {
			continue
		}

		client := NewSidecarClient(reg.SidecarURL, reg.AuthToken, reg.TimeoutMs)
		manifest := ConnectorManifest{
			ConnectorCode: reg.ConnectorCode,
			Version:       reg.Version,
			ConnectorType: "http_sidecar",
			AuthType:      "api_key",
			ReleaseStage:  "production",
		}

		var caps []ConnectorCapability
		for _, c := range reg.Capabilities {
			caps = append(caps, ConnectorCapability{
				Name:        c,
				Kind:        CapabilityKindRead,
				InputSchema: map[string]any{"type": "object"},
			})
		}

		connector := NewSidecarConnector(manifest, caps, client)
		if err := s.runtime.RegisterConnector(ctx, connector); err != nil {
			continue
		}

		s.mu.Lock()
		s.registrations[reg.ID] = &reg
		s.mu.Unlock()
	}
	return nil
}

func (s *SidecarService) GetSidecarCapabilities(reg *SidecarRegistration) []ConnectorCapability {
	var caps []ConnectorCapability
	for _, c := range reg.Capabilities {
		caps = append(caps, ConnectorCapability{
			Name:        c,
			Kind:        CapabilityKindRead,
			InputSchema: map[string]any{"type": "object"},
		})
	}
	return caps
}

func (s *SidecarService) ValidateCapabilities(ctx context.Context, reg *SidecarRegistration) error {
	client := NewSidecarClient(reg.SidecarURL, reg.AuthToken, reg.TimeoutMs)
	manifest, err := client.GetManifest(ctx)
	if err != nil {
		return fmt.Errorf("get manifest from sidecar: %w", err)
	}

	if manifest == nil {
		return fmt.Errorf("empty manifest received")
	}

	manifestCaps := make(map[string]bool)
	for _, c := range manifest.Capabilities {
		manifestCaps[c.Name] = true
	}

	for _, c := range reg.Capabilities {
		if !manifestCaps[c] {
			return fmt.Errorf("capability %s not found in sidecar manifest", c)
		}
	}
	return nil
}

func (s *SidecarService) GetMetadata(tenantID string) map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]map[string]any, 0, len(s.registrations))
	for _, reg := range s.registrations {
		if tenantID != "" && reg.TenantID != tenantID {
			continue
		}
		caps, _ := json.Marshal(reg.Capabilities)
		items = append(items, map[string]any{
			"id":            reg.ID,
			"connector_code": reg.ConnectorCode,
			"version":       reg.Version,
			"sidecar_url":   reg.SidecarURL,
			"capabilities":  json.RawMessage(caps),
			"status":        reg.Status,
			"health_status": reg.HealthStatus,
		})
	}
	return map[string]any{"sidecars": items, "total": len(items)}
}