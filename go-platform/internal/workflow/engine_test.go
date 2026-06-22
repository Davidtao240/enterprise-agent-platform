package workflow

import "testing"

func TestEngineTemplateRouting(t *testing.T) {
	engine := NewEngine()
	def, err := engine.ParseDefinition(`{
		"nodes": [
			{"id":"upload","type":"file_upload","name":"Upload"},
			{"id":"agent_graph","type":"agent_graph","name":"Analyze"},
			{"id":"human_review","type":"human_review","name":"Review"},
			{"id":"archive","type":"system","name":"Archive"}
		],
		"edges": [
			{"from":"upload","to":"agent_graph"},
			{"from":"agent_graph","to":"human_review","when":"succeeded"},
			{"from":"human_review","to":"archive","when":"approved"}
		]
	}`)
	if err != nil {
		t.Fatalf("ParseDefinition: %v", err)
	}

	entries := engine.GetEntryNodes(def)
	if len(entries) != 1 || entries[0].ID != "upload" {
		t.Fatalf("entry nodes = %#v, want upload", entries)
	}

	next := engine.GetNextNodes(def, "human_review", EdgeWhenApproved)
	if len(next) != 1 || next[0].ID != "archive" {
		t.Fatalf("approved review next nodes = %#v, want archive", next)
	}

	rejected := engine.GetNextNodes(def, "human_review", EdgeWhenRejected)
	if len(rejected) != 0 {
		t.Fatalf("rejected review next nodes = %#v, want none", rejected)
	}
}

func TestEngineStateValidation(t *testing.T) {
	engine := NewEngine()
	if err := engine.ValidateInstanceTransition(StatusWaitingReview, StatusApproved); err != nil {
		t.Fatalf("waiting_review -> approved should be valid: %v", err)
	}
	if err := engine.ValidateInstanceTransition(StatusArchived, StatusRunning); err == nil {
		t.Fatal("archived -> running should be invalid")
	}
	if err := engine.ValidateNodeTransition(NodeStatusWaitingReview, NodeStatusSucceeded); err != nil {
		t.Fatalf("waiting_review node -> succeeded should be valid: %v", err)
	}
}
