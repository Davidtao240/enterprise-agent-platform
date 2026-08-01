package agent

import (
	"encoding/json"
	"testing"
)

func TestListAgentsResponseSerializesSharedMetadata(t *testing.T) {
	payload, err := json.Marshal(ListAgentsResponse{
		AgentID:       "schema_mapping_agent",
		Name:          "Schema Mapping Agent",
		Domain:        "shared",
		ReusableScope: "shared",
		Status:        "active",
	})
	if err != nil {
		t.Fatalf("marshal agent registry response: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal agent registry response: %v", err)
	}
	if decoded["domain"] != "shared" {
		t.Fatalf("domain = %v, want shared", decoded["domain"])
	}
	if decoded["reusable_scope"] != "shared" {
		t.Fatalf(
			"reusable_scope = %v, want shared",
			decoded["reusable_scope"],
		)
	}
}
