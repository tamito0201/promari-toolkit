package model_test

import (
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

func TestWorkloadIssues(t *testing.T) {
	t.Parallel()
	jst := time.FixedZone("JST", 9*3600)
	// 2026-10-03 19:00 in Japan: a milestone due "2026-10-03" is due today.
	now := time.Date(2026, 10, 3, 19, 0, 0, 0, jst)
	due := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	items := []model.IssueItem{
		{Number: 1, Due: due(1), Updated: now.Add(-time.Hour)},                 // 2 days late
		{Number: 2, Due: due(2), Updated: now.Add(-15 * 24 * time.Hour)},       // 1 day late, stale
		{Number: 3, Due: due(3), Updated: now},                                 // due today
		{Number: 4, Due: due(10), Updated: now},                                // within a week
		{Number: 5, Due: due(11), Updated: now},                                // past the week
		{Number: 6, Created: now.Add(-49 * time.Hour), Labels: []string{"P1"}}, // urgent, undated
		{Number: 7, Created: now.Add(-47 * time.Hour), Labels: []string{"urgent"}},
		{Number: 8, Created: now.Add(-72 * time.Hour), Labels: []string{"priority:medium"}},
	}
	var w model.Workload
	w.AddIssues(items, now)
	want := model.Workload{Issues: 8, Overdue: 2, MostLate: 2, LatestIssue: 1, DueToday: 1, DueWeek: 1, Undated: 3, Stale: 1, Urgent: 1}
	if w != want {
		t.Errorf("AddIssues() = %+v\nwant %+v", w, want)
	}
}

func TestWorkloadPulls(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 3, 4, 0, 0, 0, time.UTC)
	var w model.Workload
	w.AddOpen([]model.PullItem{
		{Draft: true, Updated: now},
		{ChangesRequested: true, Updated: now.Add(-20 * 24 * time.Hour)},
	}, now)
	w.AddMerged([]model.PullItem{
		{Created: now.Add(-4 * time.Hour), Merged: now, Reviewed: true, FirstPass: true},
		{Created: now.Add(-2 * time.Hour), Merged: now, Reviewed: true},
		{Created: now.Add(-10 * time.Hour), Merged: now},
		{Created: now.Add(-time.Hour), Merged: now},
		{Created: now}, // not merged: not counted
	})
	want := model.Workload{Open: 2, Drafts: 1, MyTurn: 1, Stale: 1, Merged: 4, LeadP50: 3 * time.Hour, Reviewed: 2, FirstPass: 1}
	if w != want {
		t.Errorf("= %+v\nwant %+v", w, want)
	}
	var odd model.Workload
	odd.AddMerged([]model.PullItem{{Created: now.Add(-time.Hour), Merged: now}, {Created: now.Add(-5 * time.Hour), Merged: now}, {Created: now.Add(-9 * time.Hour), Merged: now}})
	if odd.LeadP50 != 5*time.Hour {
		t.Errorf("the median of three = %v", odd.LeadP50)
	}
}

func TestWorkloadRuns(t *testing.T) {
	t.Parallel()
	at := func(h int) time.Time { return time.Date(2026, 10, 3, h, 0, 0, 0, time.UTC) }
	var w model.Workload
	w.AddRuns([]model.RunItem{
		{Workflow: "CI", Conclusion: "", Created: at(9)}, // running: skipped
		{Workflow: "CI", Conclusion: "failure", Created: at(8)},
		{Workflow: "Docs", Conclusion: "success", Created: at(8)},
		{Workflow: "CI", Conclusion: "failure", Created: at(6)},
		{Workflow: "Lint", Conclusion: "timed_out", Created: at(7)},
		{Workflow: "CI", Conclusion: "success", Created: at(5)},
		{Workflow: "CI", Conclusion: "failure", Created: at(4)}, // before the success: not this streak
		{Workflow: "Docs", Conclusion: "failure", Created: at(3)},
	})
	if w.Red != 2 || w.RedWorkflow != "CI" || !w.RedSince.Equal(at(6)) {
		t.Errorf("AddRuns() = %d %q %v", w.Red, w.RedWorkflow, w.RedSince)
	}
	var green model.Workload
	green.AddRuns([]model.RunItem{{Workflow: "CI", Conclusion: "success", Created: at(8)}, {Workflow: "CI", Conclusion: "failure", Created: at(7)}})
	if green.Red != 0 || !green.RedSince.IsZero() {
		t.Error("a workflow whose last run passed is not red")
	}
}

func TestIsUrgentLabel(t *testing.T) {
	t.Parallel()
	for _, l := range []string{"P0", "p1", "critical", "Blocker", "incident", "priority:critical", "priority/urgent", "sev-1"} {
		if !model.IsUrgentLabel(l) {
			t.Errorf("%q is urgent", l)
		}
	}
	for _, l := range []string{"priority:medium", "priority:high", "bug", "P2", "critical-path-docs"} {
		if model.IsUrgentLabel(l) {
			t.Errorf("%q is not urgent", l)
		}
	}
}
