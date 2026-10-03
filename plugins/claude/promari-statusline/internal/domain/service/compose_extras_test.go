package service

import (
	"slices"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

// TestComposeKPIExtras covers the measures added after the KPI books: old
// branches, branch switches, a stale fetch, reverts, the repair of failing
// tests, the duration of the checks and the review rounds.
func TestComposeKPIExtras(t *testing.T) {
	t.Parallel()
	view := View{Now: now, Facts: model.Facts{
		Git: model.Some(model.Git{
			Branch: "feature/x", DefaultBranch: "main", OldBranches: 3, SwitchesToday: 4, FetchedAt: now.Add(-50 * time.Hour),
			CommitsToday: 5, RevertsToday: 1,
		}),
		Pull: model.Some(model.PullRequest{Number: 9, Passed: 4, CIDuration: 6 * time.Minute, Rounds: 2}),
		Transcript: model.Some(model.Transcript{Quality: model.Quality{
			Tests:      model.CheckRuns{Runs: 3, Passed: 2, Failed: 1, Last: model.OutcomePass},
			GreenSince: now.Add(-45 * time.Minute), LastRepair: 12 * time.Minute,
		}}),
	}}
	got := render(Compose(&view))
	for _, want := range []string{
		"Old ×3 14d+", "Switch ×4 today", "Fetched 2d2h ago", "Revert ×1 today",
		"green 45m (fixed in 12m)", "CI ✅ 4 6m", "Rounds ×2",
	} {
		if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, want) }) {
			t.Errorf("missing %q in\n%s", want, strings.Join(got, "\n"))
		}
	}

	fresh := View{Now: now, Facts: model.Facts{
		Git:  model.Some(model.Git{Branch: "feature/x", DefaultBranch: "main", FetchedAt: now.Add(-time.Hour)}),
		Pull: model.Some(model.PullRequest{Number: 9, Rounds: 1}),
	}}
	lines := strings.Join(render(Compose(&fresh)), "\n")
	if strings.Contains(lines, "Fetched") || !strings.Contains(lines, "Rounds ×1") {
		t.Errorf("a fetch of an hour ago is fresh; one round shows in grey:\n%s", lines)
	}
}

func TestComposeModelMixTie(t *testing.T) {
	t.Parallel()
	view := View{Facts: model.Facts{Transcript: model.Some(model.Transcript{Requests: 4, Models: map[string]int{"claude-sonnet-5-5": 2, "claude-haiku-4-5": 2}})}}
	lines := strings.Join(render(Compose(&view)), "\n")
	h, s := strings.Index(lines, "haiku"), strings.Index(lines, "sonnet")
	if h < 0 || s < 0 || h > s {
		t.Errorf("models used as often are shown by name:\n%s", lines)
	}
}

func TestComposeBranchAgeBetweenLimits(t *testing.T) {
	t.Parallel()
	view := View{Now: now, Facts: model.Facts{Git: model.Some(model.Git{Branch: "feature/login-ui", DefaultBranch: "main", BranchStart: now.Add(-5 * time.Hour)})}}
	groups := Compose(&view)
	for _, g := range groups {
		for _, c := range g.Chips {
			if len(c) > 0 && c[0].Text == "Branch 5h00m" && c[0].Tone != model.ToneCaution {
				t.Errorf("a branch older than %v and younger than a day is yellow: %v", model.BranchTooOld, c[0].Tone)
			}
			if len(c) > 0 && c[0].Text == "Branch 5h00m" {
				return
			}
		}
	}
	t.Errorf("no branch age in %q", render(groups))
}
