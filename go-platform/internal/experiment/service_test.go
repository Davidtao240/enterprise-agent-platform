package experiment

import (
	"testing"
)

func strPtr(s string) *string { return &s }

func TestBuildDiffReportFieldLevel(t *testing.T) {
	source := &SourceRun{
		ID: "src-1", Status: "succeeded",
		OutputSummaryJSON: strPtr(`{"amount": 100, "vendor": "acme", "kept": true}`),
	}
	replay := &RunTerminalState{
		Status: "succeeded",
		OutputSummaryJSON: strPtr(`{"amount": 200, "vendor": "acme", "added_field": "x"}`),
	}
	report := BuildDiffReport(source, replay, "rep-1")
	if report.Identical {
		t.Fatalf("fields differ; Identical must be false")
	}
	if report.SourceRunID != "src-1" || report.ReplayRunID != "rep-1" {
		t.Fatalf("run linkage wrong: %+v", report)
	}
	if f := report.Fields["amount"]; f.Status != "changed" {
		t.Fatalf("amount should be changed, got %+v", f)
	}
	if f := report.Fields["vendor"]; f.Status != "same" {
		t.Fatalf("vendor should be same, got %+v", f)
	}
	if f := report.Fields["kept"]; f.Status != "removed" {
		t.Fatalf("kept should be removed, got %+v", f)
	}
	if f := report.Fields["added_field"]; f.Status != "added" {
		t.Fatalf("added_field should be added, got %+v", f)
	}
}

func TestBuildDiffReportIdentical(t *testing.T) {
	source := &SourceRun{ID: "src-1", Status: "succeeded", OutputSummaryJSON: strPtr(`{"a":1}`)}
	replay := &RunTerminalState{Status: "succeeded", OutputSummaryJSON: strPtr(`{"a":1}`)}
	report := BuildDiffReport(source, replay, "rep-1")
	if !report.Identical {
		t.Fatalf("identical outputs must yield Identical=true")
	}
}

func TestBuildDiffReportNilOutputs(t *testing.T) {
	source := &SourceRun{ID: "src-1", Status: "succeeded", OutputSummaryJSON: nil}
	replay := &RunTerminalState{Status: "succeeded", OutputSummaryJSON: nil}
	report := BuildDiffReport(source, replay, "rep-1")
	if !report.Identical || len(report.Fields) != 0 {
		t.Fatalf("nil outputs must yield empty identical report, got %+v", report)
	}
}

func TestBuildDiffReportStatusMismatch(t *testing.T) {
	source := &SourceRun{ID: "src-1", Status: "succeeded", OutputSummaryJSON: strPtr(`{"a":1}`)}
	replay := &RunTerminalState{Status: "failed", OutputSummaryJSON: strPtr(`{"a":1}`)}
	if report := BuildDiffReport(source, replay, "rep-1"); report.Identical {
		t.Fatalf("status mismatch must not be Identical")
	}
}

func TestAscendingStages(t *testing.T) {
	cases := []struct {
		stages []int
		want   bool
	}{
		{[]int{1, 5, 20, 100}, true},
		{[]int{100}, true},
		{[]int{1, 1}, false},   // 重复
		{[]int{5, 1}, false},   // 递减
		{[]int{1, 10, 10, 100}, false},
	}
	for _, c := range cases {
		if got := ascendingStages(c.stages); got != c.want {
			t.Fatalf("ascendingStages(%v) = %v, want %v", c.stages, got, c.want)
		}
	}
}
