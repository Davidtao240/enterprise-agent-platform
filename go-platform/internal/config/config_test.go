package config

import (
	"testing"
	"time"
)

func TestLoadReadsAgentServiceURL(t *testing.T) {
	t.Setenv("AGENT_SERVICE_URL", "http://agent-service-test:8000")
	t.Setenv("INTERNAL_SERVICE_TOKEN", "runtime-test-token")

	cfg := Load()
	if cfg.AgentServiceURL != "http://agent-service-test:8000" {
		t.Fatalf("AgentServiceURL = %q", cfg.AgentServiceURL)
	}
	if cfg.InternalServiceToken != "runtime-test-token" {
		t.Fatalf("InternalServiceToken was not loaded")
	}
}

func TestLoadReadsAgentRunStaleAfter(t *testing.T) {
	t.Setenv("AGENT_RUN_STALE_AFTER", "3m")
	if got := Load().AgentRunStaleAfter; got != 3*time.Minute {
		t.Fatalf("AgentRunStaleAfter = %v, want 3m", got)
	}
}

func TestLoadAgentRunStaleAfterDefaultsWhenUnsetOrInvalid(t *testing.T) {
	t.Setenv("AGENT_RUN_STALE_AFTER", "")
	if got := Load().AgentRunStaleAfter; got != 10*time.Minute {
		t.Fatalf("default AgentRunStaleAfter = %v, want 10m", got)
	}
	t.Setenv("AGENT_RUN_STALE_AFTER", "not-a-duration")
	if got := Load().AgentRunStaleAfter; got != 10*time.Minute {
		t.Fatalf("invalid AgentRunStaleAfter = %v, want fallback 10m", got)
	}
}

func TestLoadWorkerRuntimeV2(t *testing.T) {
	t.Setenv("WORKER_RUNTIME_V2", "")
	if Load().WorkerRuntimeV2 {
		t.Fatal("default WorkerRuntimeV2 = true, want false")
	}
	t.Setenv("WORKER_RUNTIME_V2", "true")
	if !Load().WorkerRuntimeV2 {
		t.Fatal("WorkerRuntimeV2 was not enabled by WORKER_RUNTIME_V2=true")
	}
}
