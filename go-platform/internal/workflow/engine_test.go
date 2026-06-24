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

// TestEngineAllInstanceTransitions verifies every allowed and disallowed instance state transition.
func TestEngineAllInstanceTransitions(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		from, to string
		valid    bool
	}{
		// draft transitions
		{StatusDraft, StatusRunning, true},
		{StatusDraft, StatusCancelled, true},
		{StatusDraft, StatusArchived, false},
		{StatusDraft, StatusFailed, false},
		{StatusDraft, StatusApproved, false},
		// running transitions
		{StatusRunning, StatusWaitingReview, true},
		{StatusRunning, StatusFailed, true},
		{StatusRunning, StatusCancelled, true},
		{StatusRunning, StatusApproved, false},
		{StatusRunning, StatusArchived, false},
		{StatusRunning, StatusDraft, false},
		// waiting_review transitions
		{StatusWaitingReview, StatusApproved, true},
		{StatusWaitingReview, StatusRejected, true},
		{StatusWaitingReview, StatusCancelled, true},
		{StatusWaitingReview, StatusRunning, false},
		{StatusWaitingReview, StatusArchived, false},
		// approved transitions
		{StatusApproved, StatusArchived, true},
		{StatusApproved, StatusRunning, false},
		{StatusApproved, StatusRejected, false},
		// terminal states
		{StatusRejected, StatusRunning, false},
		{StatusRejected, StatusArchived, false},
		{StatusArchived, StatusRunning, false},
		{StatusFailed, StatusRunning, false},
		{StatusCancelled, StatusRunning, false},
		// unknown state
		{"bogus", StatusRunning, false},
	}

	for _, tc := range tests {
		err := engine.ValidateInstanceTransition(tc.from, tc.to)
		if tc.valid && err != nil {
			t.Errorf("expected %s -> %s to be valid, got error: %v", tc.from, tc.to, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("expected %s -> %s to be invalid, got nil", tc.from, tc.to)
		}
	}
}

// TestEngineAllNodeTransitions verifies every allowed and disallowed node state transition.
func TestEngineAllNodeTransitions(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		from, to string
		valid    bool
	}{
		// pending transitions
		{NodeStatusPending, NodeStatusRunning, true},
		{NodeStatusPending, NodeStatusSkipped, true},
		{NodeStatusPending, NodeStatusCancelled, true},
		{NodeStatusPending, NodeStatusSucceeded, false},
		{NodeStatusPending, NodeStatusFailed, false},
		// running transitions
		{NodeStatusRunning, NodeStatusSucceeded, true},
		{NodeStatusRunning, NodeStatusFailed, true},
		{NodeStatusRunning, NodeStatusWaitingReview, true},
		{NodeStatusRunning, NodeStatusCancelled, true},
		{NodeStatusRunning, NodeStatusPending, false},
		// waiting_review transitions
		{NodeStatusWaitingReview, NodeStatusSucceeded, true},
		{NodeStatusWaitingReview, NodeStatusFailed, true},
		{NodeStatusWaitingReview, NodeStatusCancelled, true},
		{NodeStatusWaitingReview, NodeStatusRunning, false},
		// failed → running (retry)
		{NodeStatusFailed, NodeStatusRunning, true},
		{NodeStatusFailed, NodeStatusSucceeded, false},
		// terminal states
		{NodeStatusSucceeded, NodeStatusRunning, false},
		{NodeStatusSkipped, NodeStatusRunning, false},
		{NodeStatusCancelled, NodeStatusRunning, false},
		// unknown state
		{"bogus", NodeStatusRunning, false},
	}

	for _, tc := range tests {
		err := engine.ValidateNodeTransition(tc.from, tc.to)
		if tc.valid && err != nil {
			t.Errorf("expected node %s -> %s to be valid, got error: %v", tc.from, tc.to, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("expected node %s -> %s to be invalid, got nil", tc.from, tc.to)
		}
	}
}

// TestEngineCanRetryNode tests retry boundary conditions.
func TestEngineCanRetryNode(t *testing.T) {
	engine := NewEngine()

	// failed node with retries remaining — should succeed
	node := &NodeInstance{Status: NodeStatusFailed, RetryCount: 0, MaxRetries: 3}
	if err := engine.CanRetryNode(node); err != nil {
		t.Fatalf("expected retryable, got error: %v", err)
	}

	// failed node at max retries — should fail
	node = &NodeInstance{Status: NodeStatusFailed, RetryCount: 3, MaxRetries: 3}
	if err := engine.CanRetryNode(node); err == nil {
		t.Fatal("expected retry exhausted, got nil")
	}

	// non-failed node — should fail
	node = &NodeInstance{Status: NodeStatusRunning, RetryCount: 0, MaxRetries: 3}
	if err := engine.CanRetryNode(node); err == nil {
		t.Fatal("expected cannot retry non-failed node, got nil")
	}

	// succeeded node — should fail
	node = &NodeInstance{Status: NodeStatusSucceeded, RetryCount: 0, MaxRetries: 3}
	if err := engine.CanRetryNode(node); err == nil {
		t.Fatal("expected cannot retry succeeded node, got nil")
	}
}

// TestEngineCanStartInstance tests start preconditions.
func TestEngineCanStartInstance(t *testing.T) {
	engine := NewEngine()

	// draft — ok
	if err := engine.CanStartInstance(&Instance{Status: StatusDraft}); err != nil {
		t.Fatalf("draft should be startable: %v", err)
	}

	// non-draft states — not ok
	for _, s := range []string{StatusRunning, StatusWaitingReview, StatusApproved, StatusRejected, StatusArchived, StatusFailed, StatusCancelled} {
		if err := engine.CanStartInstance(&Instance{Status: s}); err == nil {
			t.Errorf("status %s should not be startable", s)
		}
	}
}

// TestEngineCanCancelInstance tests cancellation preconditions.
func TestEngineCanCancelInstance(t *testing.T) {
	engine := NewEngine()

	// cancellable states
	for _, s := range []string{StatusDraft, StatusRunning, StatusWaitingReview, StatusFailed, StatusRejected, StatusApproved} {
		if err := engine.CanCancelInstance(&Instance{Status: s}); err != nil {
			t.Errorf("status %s should be cancellable, got error: %v", s, err)
		}
	}

	// non-cancellable states
	for _, s := range []string{StatusArchived, StatusCancelled} {
		if err := engine.CanCancelInstance(&Instance{Status: s}); err == nil {
			t.Errorf("status %s should not be cancellable", s)
		}
	}
}

// TestEngineGetNextNodesWithEdgeWhen tests various edge conditions.
func TestEngineGetNextNodesWithEdgeWhen(t *testing.T) {
	engine := NewEngine()
	def, _ := engine.ParseDefinition(`{
		"nodes": [
			{"id":"a","type":"agent_graph","name":"A"},
			{"id":"b","type":"human_review","name":"B"},
			{"id":"c","type":"system","name":"C"},
			{"id":"d","type":"system","name":"D"}
		],
		"edges": [
			{"from":"a","to":"b","when":"succeeded"},
			{"from":"a","to":"c","when":"failed"},
			{"from":"b","to":"d","when":"approved"},
			{"from":"b","to":"a","when":"rejected"}
		]
	}`)

	// a succeeded → b
	next := engine.GetNextNodes(def, "a", EdgeWhenSucceeded)
	if len(next) != 1 || next[0].ID != "b" {
		t.Fatalf("a succeeded → expected [b], got %v", next)
	}

	// a failed → c
	next = engine.GetNextNodes(def, "a", EdgeWhenFailed)
	if len(next) != 1 || next[0].ID != "c" {
		t.Fatalf("a failed → expected [c], got %v", next)
	}

	// b approved → d
	next = engine.GetNextNodes(def, "b", EdgeWhenApproved)
	if len(next) != 1 || next[0].ID != "d" {
		t.Fatalf("b approved → expected [d], got %v", next)
	}

	// b rejected → a
	next = engine.GetNextNodes(def, "b", EdgeWhenRejected)
	if len(next) != 1 || next[0].ID != "a" {
		t.Fatalf("b rejected → expected [a], got %v", next)
	}

	// c has no outgoing edges → empty
	next = engine.GetNextNodes(def, "c", EdgeWhenSucceeded)
	if len(next) != 0 {
		t.Fatalf("c → expected empty, got %v", next)
	}
}

// TestEngineGetNextNodesEmptyEdgeWhen tests that empty edge "when" matches any result.
func TestEngineGetNextNodesEmptyEdgeWhen(t *testing.T) {
	engine := NewEngine()
	def, _ := engine.ParseDefinition(`{
		"nodes": [
			{"id":"a","type":"file_upload","name":"A"},
			{"id":"b","type":"agent_graph","name":"B"}
		],
		"edges": [
			{"from":"a","to":"b"}
		]
	}`)

	// edge without "when" should match any edgeWhen
	for _, when := range []string{EdgeWhenSucceeded, EdgeWhenFailed, EdgeWhenApproved, EdgeWhenRejected} {
		next := engine.GetNextNodes(def, "a", when)
		if len(next) != 1 || next[0].ID != "b" {
			t.Errorf("a (no when) with edgeWhen=%s → expected [b], got %v", when, next)
		}
	}
}

// TestEngineFindNodeByID tests node lookup in template.
func TestEngineFindNodeByID(t *testing.T) {
	engine := NewEngine()
	def, _ := engine.ParseDefinition(`{
		"nodes": [
			{"id":"upload","type":"file_upload","name":"Upload"},
			{"id":"agent","type":"agent_graph","name":"Agent"}
		],
		"edges": []
	}`)

	// existing node
	n, err := engine.FindNodeByID(def, "upload")
	if err != nil {
		t.Fatalf("FindNodeByID upload: %v", err)
	}
	if n.Type != "file_upload" {
		t.Fatalf("expected type file_upload, got %s", n.Type)
	}

	// non-existing node
	_, err = engine.FindNodeByID(def, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent node, got nil")
	}
}

// TestEngineParseDefinitionInvalid tests invalid JSON handling.
func TestEngineParseDefinitionInvalid(t *testing.T) {
	engine := NewEngine()
	_, err := engine.ParseDefinition(`{invalid json}`)
	if err == nil {
		t.Fatal("expected parse error for invalid JSON, got nil")
	}
}

// TestEngineGetEntryNodesMultiple tests template with multiple entry nodes.
func TestEngineGetEntryNodesMultiple(t *testing.T) {
	engine := NewEngine()
	def, _ := engine.ParseDefinition(`{
		"nodes": [
			{"id":"a","type":"file_upload","name":"A"},
			{"id":"b","type":"file_upload","name":"B"},
			{"id":"c","type":"agent_graph","name":"C"}
		],
		"edges": [
			{"from":"a","to":"c"},
			{"from":"b","to":"c"}
		]
	}`)

	entries := engine.GetEntryNodes(def)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entry nodes, got %d", len(entries))
	}
	ids := map[string]bool{}
	for _, e := range entries {
		ids[e.ID] = true
	}
	if !ids["a"] || !ids["b"] {
		t.Fatalf("expected entries a and b, got %v", ids)
	}
}
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
