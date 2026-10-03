package service_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
)

func TestJoinSubagentRuns(t *testing.T) {
	decision := func(id string, c model.Class) model.Entry {
		return model.Entry{Event: model.EventSubagent, ToolUseID: id, Class: c}
	}
	result := func(id string) model.Entry { return model.Entry{Event: model.EventSubagentResult, ToolUseID: id} }
	tests := []struct {
		name    string
		entries []model.Entry
		want    []service.SubagentRun
	}{
		{name: "nothing", want: nil},
		{
			name:    "parallel subagents join by tool_use_id, not by order",
			entries: []model.Entry{decision("a", model.ClassLookup), decision("b", model.ClassComplex), result("b"), result("a")},
			want: []service.SubagentRun{
				{Result: result("b"), Decision: decision("b", model.ClassComplex), Routed: true},
				{Result: result("a"), Decision: decision("a", model.ClassLookup), Routed: true},
			},
		},
		{
			name:    "a result without a decision is not routed",
			entries: []model.Entry{{Event: model.EventPrompt}, result("x")},
			want:    []service.SubagentRun{{Result: result("x")}},
		},
		{
			name:    "an empty id never joins, even to an empty-id decision",
			entries: []model.Entry{decision("", model.ClassLookup), result("")},
			want:    []service.SubagentRun{{Result: result("")}},
		},
		{
			name:    "a later decision with the same id wins",
			entries: []model.Entry{decision("a", model.ClassLookup), decision("a", model.ClassStandard), result("a")},
			want:    []service.SubagentRun{{Result: result("a"), Decision: decision("a", model.ClassStandard), Routed: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.JoinSubagentRuns(tt.entries)); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestSubagentRunEvidence(t *testing.T) {
	routed := func(c model.Class, s model.SubagentStatus) service.SubagentRun {
		return service.SubagentRun{Routed: true, Decision: model.Entry{Class: c}, Result: model.Entry{Status: s}}
	}
	tests := []struct {
		name string
		run  service.SubagentRun
		want bool
	}{
		{name: "a routed run that completed", run: routed(model.ClassLookup, model.StatusCompleted), want: true},
		{name: "a routed run that failed", run: routed(model.ClassLookup, "error"), want: true},
		{name: "a run launched in the background has no outcome yet", run: routed(model.ClassLookup, model.StatusAsyncLaunched)},
		{name: "an abstained run has no class to learn", run: routed(model.ClassNone, model.StatusCompleted)},
		{name: "a run without its decision", run: service.SubagentRun{Result: model.Entry{Status: model.StatusCompleted}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.run.Evidence(); got != tt.want {
				t.Errorf("Evidence() = %v, want %v", got, tt.want)
			}
		})
	}
}
