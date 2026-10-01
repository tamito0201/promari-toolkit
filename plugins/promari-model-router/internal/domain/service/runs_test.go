package service_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
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
