package service

import (
	"slices"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

// The groups every view shows: the model and the clock.
const (
	env    = "🧭 Env: ?"
	system = "💻 System: 🕐 04:09"
)

// TestComposeMetrics covers the chips read from the transcript, the fields of
// the session report added in 1.4.0, and the measures derived from research.
func TestComposeMetrics(t *testing.T) {
	t.Parallel()
	full := model.Transcript{
		Tools:   model.ToolStats{Total: 30, Errors: 1, Top: []model.ToolCount{{Name: "Bash", Count: 30}}},
		Tokens:  model.TokenTotals{Input: 1000, CacheWrite: 9000, CacheRead: 90_000, Output: 4000, Thinking: 1000},
		Prompts: 4, Interrupts: 1, Denials: 1, Refusals: 1, Truncated: 2, WebSearches: 3, WebFetches: 1,
		Requests: 40, SideRequests: 6,
		Models:          map[string]int{"claude-opus-5-5": 30, "claude-haiku-4-5-20251001": 10},
		ThinkingSeconds: 150,
		Turns:           []float64{30, 90, 600, 45},
		Compactions:     2, LastCompaction: now.Add(-40 * time.Minute),
		Files: []string{"/a", "/b"},
		Hooks: 9, HookErrors: 1, Queued: 2, Diagnostics: 3, PermissionMode: "auto",
	}
	tests := []struct {
		name string
		view View
		want []string
	}{
		{
			"the transcript: tokens over the session, agent, turns, files",
			View{Facts: model.Facts{Transcript: model.Some(full)}},
			[]string{
				"🚀 Perf: Turn 45s (p50 45s · p90 10m) | Think time 2m | ErrRate 3.3%",
				"📊 Tokens: Σ In 100k / Out 4k (cached 90%) | Think share 25% | Req ×40 | compact ×2 (40m ago)",
				"🔧 Work: Tools ×30 Bash30 ❌ Err 1 | Edited 2 files | Hooks ×9 ❌ 1 | Diag ×3",
				"🤝 Agent: Prompts ×4 | Auto ×7.5/prompt | Interv 50% (✋1 ⛔1) | Refuse ×1 | MaxTok ×2 | Queued ×2 | Web search 3 fetch 1 | Sub ×6 req | Models opus-5-5 75% · haiku-4-5-2… 25%",
				"🧭 Env: ? | Perm auto", system,
			},
		},
		{
			"a transcript without prompts, thinking or turns shows what it has",
			View{Facts: model.Facts{Transcript: model.Some(model.Transcript{Requests: 1, Tokens: model.TokenTotals{CacheRead: 10}, Models: map[string]int{"a": 1}})}},
			[]string{"📊 Tokens: Σ In 10 / Out 0 (cached 100%) | Req ×1", env, system},
		},
		{
			"the compactions guessed from the context when the transcript has none",
			View{Activity: model.Some(model.Activity{Compactions: 1, StreakStart: now, LastSeen: now})},
			[]string{"🔥 Burn: Active 0m | Streak 0m", "📊 Tokens: compact ×1", env, system},
		},
		{
			"the last request's input split, and its output",
			View{Session: model.Session{Context: model.ContextWindow{Current: 30_000, Fresh: 2000, Written: 8000, Read: 20_000, TotalOutput: 500}}},
			[]string{"📊 Tokens: Last new 2k wr 8k rd 20k → out 500", env, system},
		},
		{
			"the prompt cache in full",
			View{Session: model.Session{Cache: model.PromptCache{
				HitRatio: model.Some(0.5), TTL: "5m", Warm: model.Some(false), WriteTokens: 50_000, MissTokens: 20_000,
				Rebuilds: 2, MissCauses: map[string]int{"tools_changed": 1, "ttl_expired_5m": 3, "a_cause": 1},
			}}},
			[]string{
				"📦 Cache: Hit 50% TTL 5m cold | Save 45% | Write 50k (miss 20k) | Rebuild ×2 | Causes ttl_expired_5m×3 a_cause×1 tools_changed×1",
				env, system,
			},
		},
		{
			"caching that no response reported",
			View{Session: model.Session{Cache: model.PromptCache{Observed: model.Some(false), HitRatio: model.Some(0.0)}}},
			[]string{"📦 Cache: Caching off", env, system},
		},
		{
			"the last miss with its cause and age",
			View{Session: model.Session{Cache: model.PromptCache{
				Observed: model.Some(true), HitRatio: model.Some(0.9), Misses: 1, LastMissCause: "tools_changed", LastMissAt: now.Add(-12 * time.Minute),
			}}},
			[]string{"📦 Cache: Hit 90% 1 Miss (tools_changed 12m ago) | Save 81%", env, system},
		},
		{
			"deep work: a finished deep streak, the current one, breaks",
			View{Activity: model.Some(model.Activity{
				WorkedSeconds: 3600, IdledSeconds: 600, Breaks: 2, DeepSeconds: 1800, LongestSeconds: 1800,
				StreakStart: now.Add(-25 * time.Minute), LastSeen: now,
			})},
			[]string{
				"🔥 Burn: Active 1h00m | Streak 25m | Idle 10m",
				"📈 KPI: Focus 86% | Deep 55m | Longest 30m | Breaks ×2",
				env, system,
			},
		},
		{
			"git: worktree, an operation, conflicts, staged, new, the diff and today's commits",
			View{
				Session: model.Session{Worktree: model.Some(model.Worktree{Name: "feature-x"})},
				Facts: model.Facts{Git: model.Some(model.Git{
					Branch: "develop", Changed: 4, Staged: 1, Untracked: 1, Conflicts: 1, Inserted: 1200, Deleted: 30,
					Operation: "rebase", CommitsToday: 3, LastCommit: now.Add(-time.Hour),
				})},
			},
			[]string{
				"🌿 Git: develop | WT feature-x | REBASE in progress | Conflict ×1 | 📝 4 Files | ➕ 1 Staged | ❓ 1 New | Diff +1,200 -30 | 📅 Cmt 1h00m (×3 today)",
				env, system,
			},
		},
		{
			"a linked git worktree outside a worktree session",
			View{Session: model.Session{GitWorktree: "linked"}, Facts: model.Facts{Git: model.Some(model.Git{Branch: "x"})}},
			[]string{"🌿 Git: x | WT linked", env, system},
		},
		{
			"a pull request with its size, age, draft and conflicts",
			View{Facts: model.Facts{
				Git:  model.Some(model.Git{Branch: "x"}),
				Pull: model.Some(model.PullRequest{Number: 9, Draft: true, Conflicts: true, Additions: 300, Deletions: 200, Files: 7, Created: now.Add(-50 * time.Hour)}),
			}},
			[]string{"🌿 Git: x", "🔀 PR: #9 draft | conflicts | Size +300 -200 7f | Age 2d2h", env, system},
		},
		{
			"the pull request Claude Code found when gh found none",
			View{
				Session: model.Session{PR: model.Some(model.SessionPR{Number: 4, ReviewState: "changes_requested", MergeRequest: true})},
				Facts:   model.Facts{Git: model.Some(model.Git{Branch: "x"})},
			},
			[]string{"🌿 Git: x", "🔀 PR: !4 changes", env, system},
		},
		{
			"env: agent, vim, a session that moved and added directories",
			View{Session: model.Session{Agent: "reviewer", Vim: "INSERT", Dir: "/work/app/sub", ProjectDir: "/work/app/", AddedDirs: 2}},
			[]string{"🧭 Env: ? | Agent reviewer | Vim INSERT | cd sub | +2 dirs", system},
		},
		{
			"a session that moved out of where it started",
			View{Session: model.Session{Dir: "/elsewhere", ProjectDir: "/work"}},
			[]string{"🧭 Env: ? | cd /elsewhere", system},
		},
		{
			"the dollars of a spend limit",
			View{
				Session: model.Session{SpendUSD: model.Some(model.SpendMoney{Used: model.Some(314.12), Limit: model.Some(500.0), Period: "monthly"})},
				Limits:  model.RateLimits{Spend: model.Some(model.RateWindow{UsedPct: 63})},
			},
			[]string{": ⚡ Claude Spend ███░░ 63% $314/$500 mo", env, system},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			view := tt.view
			view.Now = now
			if got := render(Compose(&view)); !slices.Equal(got, tt.want) {
				t.Errorf("Compose() =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(tt.want, "\n  "))
			}
		})
	}
}

func TestPullRequestStatesFromTheSession(t *testing.T) {
	t.Parallel()
	for state, want := range map[string]string{"approved": "#1 approved", "pending": "#1 review", "draft": "#1 draft", "": "#1"} {
		view := View{
			Now:     now,
			Session: model.Session{PR: model.Some(model.SessionPR{Number: 1, ReviewState: state})},
			Facts:   model.Facts{Git: model.Some(model.Git{Branch: "x"})},
		}
		if got := render(Compose(&view))[1]; got != "🔀 PR: "+want {
			t.Errorf("state %q: %s", state, got)
		}
	}
}

func TestPermissionTone(t *testing.T) {
	t.Parallel()
	for mode, want := range map[string]model.Tone{
		"bypassPermissions": model.ToneDanger, "acceptEdits": model.ToneCaution, "auto": model.ToneCaution,
		"default": model.ToneMuted, "plan": model.ToneMuted,
	} {
		if got := permissionTone(mode); got != want {
			t.Errorf("permissionTone(%s) = %v, want %v", mode, got, want)
		}
	}
}

func TestReviewTone(t *testing.T) {
	t.Parallel()
	for lines, want := range map[int]model.Tone{400: model.ToneGood, 401: model.ToneCaution, 1000: model.ToneCaution, 1001: model.ToneDanger} {
		if got := reviewTone(lines); got != want {
			t.Errorf("reviewTone(%d) = %v, want %v", lines, got, want)
		}
	}
}

func TestSpendDollars(t *testing.T) {
	t.Parallel()
	tests := []struct {
		money model.SpendMoney
		want  string
	}{
		{model.SpendMoney{Used: model.Some(5.0), Limit: model.Some(20.0), Period: "daily"}, "$5/$20 day"},
		{model.SpendMoney{Limit: model.Some(100.0), Period: "weekly"}, "$100 wk"},
		{model.SpendMoney{Used: model.Some(7.0), Period: "quarterly"}, "$7 quarterly"},
	}
	for _, tt := range tests {
		if got := spendDollars(tt.money); got != tt.want {
			t.Errorf("spendDollars(%+v) = %q, want %q", tt.money, got, tt.want)
		}
	}
}

func TestSignedAndBrief(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]string{1234: "+1,234", -56: "-56", 0: "0"} {
		if got := signed(n); got != want {
			t.Errorf("signed(%d) = %q", n, got)
		}
	}
	for d, want := range map[time.Duration]string{0: "0s", 59 * time.Second: "59s", time.Minute: "1m", -time.Second: "0s"} {
		if got := brief(d); got != want {
			t.Errorf("brief(%v) = %q", d, got)
		}
	}
}
