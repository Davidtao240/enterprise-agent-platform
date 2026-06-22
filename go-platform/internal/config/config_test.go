package config

import "testing"

func TestLoadReadsAgentServiceURL(t *testing.T) {
	t.Setenv("AGENT_SERVICE_URL", "http://agent-service-test:8000")

	cfg := Load()
	if cfg.AgentServiceURL != "http://agent-service-test:8000" {
		t.Fatalf("AgentServiceURL = %q", cfg.AgentServiceURL)
	}
}
