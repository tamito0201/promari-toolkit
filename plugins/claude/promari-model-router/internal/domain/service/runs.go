package service

import (
	"promari-model-router/internal/domain/model"
)

// SubagentRun is one subagent result with the routing decision it belongs to.
type SubagentRun struct {
	Result   model.Entry
	Decision model.Entry
	// Routed is false when no decision with the result's tool_use_id exists
	// (a result from before the ledger window, or a call the hook never saw).
	Routed bool
}

// JoinSubagentRuns pairs every subagent result in entries with its decision
// by tool_use_id: never by "the latest decision in the session", which mixes
// up subagents running in parallel. A later decision with the same id wins.
// Results are returned in ledger order.
func JoinSubagentRuns(entries []model.Entry) []SubagentRun {
	decisions := map[string]model.Entry{}
	for i := range entries {
		if e := &entries[i]; e.Event == model.EventSubagent && e.ToolUseID != "" {
			decisions[e.ToolUseID] = *e
		}
	}
	var runs []SubagentRun
	for i := range entries {
		r := &entries[i]
		if r.Event != model.EventSubagentResult {
			continue
		}
		d, ok := decisions[r.ToolUseID]
		runs = append(runs, SubagentRun{Result: *r, Decision: d, Routed: ok && r.ToolUseID != ""})
	}
	return runs
}
