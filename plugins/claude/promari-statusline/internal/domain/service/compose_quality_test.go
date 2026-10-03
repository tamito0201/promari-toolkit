package service

import (
	"slices"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

// TestComposeQuality covers the 🧪 Quality category: the checks and edits of
// the session and the risk of the uncommitted change.
func TestComposeQuality(t *testing.T) {
	t.Parallel()
	red := model.Quality{
		Tests:  model.CheckRuns{Runs: 5, Passed: 2, Failed: 2, Masked: 1, Last: model.OutcomeFail},
		Builds: model.CheckRuns{Runs: 3, Passed: 3, Last: model.OutcomePass},
		Edits:  9, EditFailures: 2, EditFailStreak: 1, Repeats: 2,
		Unverified: []string{"/w/a.go", "/w/b.go"}, RedCalls: 14, Claims: 1,
		RedSince: now.Add(-30 * time.Minute),
	}
	change := model.Git{
		Branch: "feature", CommitsToday: 5, FixesToday: 2, AICommitsToday: 4, DebtAdded: 3, DebtRemoved: 1,
		SkipsAdded: 1, AssertsAdded: 1, AssertsRemoved: 3, MocksAdded: 2, DepsAdded: 1,
		Changes: []model.FileChange{{Path: "a/x.go", Added: 30}, {Path: "a/y_test.go", Added: 20}, {Path: "b/z.go", Added: 10}},
	}
	tests := []struct {
		name   string
		view   View
		want   string
		alarms []string
	}{
		{
			"red tests for longer than most repairs take, and a change that admits debt",
			View{Now: now, Facts: model.Facts{Transcript: model.Some(model.Transcript{Quality: red}), Git: model.Some(change)}},
			"🧪 Quality: Tests ×5 ❌ (fail 50% · piped 1) red 30m · 14 calls | Build ×3 ✅ | Claim≠ ×1 | Untested 2 files | EditFail ×2 (streak 1) | Repeat ×2 | Spread 3f · 2d · 2s H0.92 | TestDiff 33% | Debt +3 -1 | Weaken skip +1 assert -2 | Mocks +2 | Deps +1 | Fix 2/5 today | AI 4/5 today",
			[]string{"Tests ×5 ❌ (fail 50% · piped 1) red 30m · 14 calls"},
		},
		{
			"tests red for less long, a run of unknown outcome, debt paid back, no test in the change",
			View{Now: now, Facts: model.Facts{
				Transcript: model.Some(model.Transcript{Quality: model.Quality{
					Tests:  model.CheckRuns{Runs: 2, Failed: 1, Last: model.OutcomeFail},
					Builds: model.CheckRuns{Runs: 1, Last: model.OutcomeUnknown}, EditFailures: 1, RedSince: now.Add(-5 * time.Minute),
				}}),
				Git: model.Some(model.Git{Branch: "x", DebtRemoved: 2, SkipsAdded: 1, AssertsRemoved: 1, AssertsAdded: 1, Changes: []model.FileChange{{Path: "x.go", Added: 3}, {Path: "README.md", Added: 9}}}),
			}},
			"🧪 Quality: Tests ×2 ❌ (fail 100%) red 5m · 0 calls | Build ×1 ? | EditFail ×1 | Spread 1f · 1d · 1s | TestDiff 0% | Debt +0 -2 | Weaken skip +1",
			nil,
		},
		{
			"passing tests and nothing else",
			View{Now: now, Facts: model.Facts{Transcript: model.Some(model.Transcript{Quality: model.Quality{Tests: model.CheckRuns{Runs: 1, Passed: 1, Last: model.OutcomePass}}})}},
			"🧪 Quality: Tests ×1 ✅",
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			groups := Compose(&tt.view)
			got := render(groups)
			i := slices.IndexFunc(got, func(s string) bool { return strings.HasPrefix(s, "🧪 Quality") })
			if i < 0 || got[i] != tt.want {
				t.Errorf("Compose() =\n%s\nwant\n%s", strings.Join(got, "\n"), tt.want)
			}
			if a := alarms(groups); !slices.Equal(a, tt.alarms) {
				t.Errorf("alarms = %q, want %q", a, tt.alarms)
			}
		})
	}
	if groups := Compose(&View{Now: now}); slices.ContainsFunc(render(groups), func(s string) bool { return strings.HasPrefix(s, "🧪") }) {
		t.Error("a view without a transcript or a repository has no quality")
	}
}

// TestComposeTrace covers the 🧬 Trace category.
func TestComposeTrace(t *testing.T) {
	t.Parallel()
	trace := model.Trace{
		Explores: 30, Rereads: 3, Researches: 1, MaxEditRun: 6,
		ObservedBytes: 200_000, LargestObserved: 40_000, PromptsSinceCompact: 7,
		TurnTokens: []float64{100_000, 50_000, 1_500_000},
	}
	tests := []struct {
		name string
		view View
		want string
	}{
		{
			"everything, with the context known",
			View{Usage: model.Some(model.ContextUsage{Used: 100_000, Size: 200_000, Pct: 50}), Facts: model.Facts{Transcript: model.Some(model.Transcript{
				Compactions: 1, Quality: model.Quality{Edits: 20}, Trace: trace,
			})}},
			"🧬 Trace: Explore 1.5/edit | Reread ×4 (read 3 · search 1) | EditRun max 6 | Obs ≈50k (50% ctx) max 10k | Since compact 7 prompts | PromptTok p50 100k max 1.50M (×15)",
		},
		{
			"no edit, no compaction, one prompt, the context unknown",
			View{Facts: model.Facts{Transcript: model.Some(model.Transcript{Trace: model.Trace{Explores: 3, MaxEditRun: 1, ObservedBytes: 400, LargestObserved: 400, PromptsSinceCompact: 1, TurnTokens: []float64{9}}})}},
			"🧬 Trace: Obs ≈100 max 100",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := render(Compose(&tt.view))
			i := slices.IndexFunc(got, func(s string) bool { return strings.HasPrefix(s, "🧬 Trace") })
			if i < 0 || got[i] != tt.want {
				t.Errorf("Compose() =\n%s\nwant\n%s", strings.Join(got, "\n"), tt.want)
			}
		})
	}
}
