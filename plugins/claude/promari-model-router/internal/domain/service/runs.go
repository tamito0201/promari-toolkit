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

// Evidence reports whether the run says how well its tier did on its class,
// so that the learned posteriors may count it: it was routed with a class,
// and it ran to an end. A subagent launched in the background has a result
// when it starts (Claude Code runs PostToolUse then), with no outcome and no
// usage: counted, it would read as a failure that used no tokens.
func (r SubagentRun) Evidence() bool {
	return r.Routed && r.Decision.Class != model.ClassNone && !r.Result.Status.Background()
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
