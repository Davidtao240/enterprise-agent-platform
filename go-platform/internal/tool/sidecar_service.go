package tool

import (
	"context"
	"encoding/json"
	"fmt"
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

func (s *SidecarService) RegisterSidecar(ctx context.Context, reg *SidecarRegistration) (*SidecarRegistration, error) {
	reg.ID = uuid.NewString()

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

func (s *SidecarService) DeregisterSidecar(ctx context.Context, id string) error {
	if err := s.repo.UpdateStatus(ctx, id, "inactive"); err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.registrations, id)
	s.mu.Unlock()
	return nil
}

func (s *SidecarService) HealthCheck(ctx context.Context, id string) (*SidecarRegistration, error) {
	s.mu.RLock()
	reg, ok := s.registrations[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("sidecar not found")
	}

	client := NewSidecarClient(reg.SidecarURL, reg.AuthToken, reg.TimeoutMs)
	resp, err := client.Health(ctx)
	if err != nil {
		_ = s.repo.UpdateHealth(ctx, id, "unhealthy", err.Error())
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
	_ = s.repo.UpdateHealth(ctx, id, healthStatus, errMsg)

	s.mu.Lock()
	s.registrations[id].HealthStatus = healthStatus
	s.registrations[id].LastError = errMsg
	s.registrations[id].UpdatedAt = time.Now()
	current := s.registrations[id]
	s.mu.Unlock()

	return current, nil
}

func (s *SidecarService) GetSidecar(ctx context.Context, id string) (*SidecarRegistration, error) {
	s.mu.RLock()
	reg, ok := s.registrations[id]
	s.mu.RUnlock()
	if ok {
		return reg, nil
	}

	rows, err := s.repo.List(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("sidecar not found")
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

func (s *SidecarService) GetMetadata() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]map[string]any, 0, len(s.registrations))
	for _, reg := range s.registrations {
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