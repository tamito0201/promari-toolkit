package service

import (
	"slices"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

// This file replays a status line psl drew in production (2026-10-05 11:06
// JST, 65-66 columns) and pins down, chip by chip, what made every number of
// that screen right. The view below is the screen read backwards: each field
// was solved from the rendered text, and the cross-checks at the bottom are
// the identities the real screen satisfied (the last request's tokens summed
// to the context, the issue counts summed to the total, and so on). A change
// that breaks one of these breaks a value somebody validated against reality.
var replayNow = time.Date(2026, 10, 5, 11, 6, 0, 0, time.FixedZone("JST", 9*60*60))

func replayView() View {
	return View{
		Now: replayNow,
		Session: model.Session{
			Reported: true,
			Name:     "Claideプラグインテストカバレッジ実装",
			Model:    "Fable 5", Effort: "high", Thinking: true,
			Repo: "example-portal", Style: "Explanatory", Version: "2.1.289",
			Cost: model.Cost{
				TotalUSD: model.Some(4.41),
				Wall:     5 * time.Minute,
				API:      90 * time.Second,
			},
			Context: model.ContextWindow{
				Size: 1_000_000, Current: 187_002,
				Fresh: 2, Written: 2_000, Read: 185_000,
				TotalOutput: 201,
			},
			Cache: model.PromptCache{
				HitRatio: model.Some(0.85), TTL: "1h",
				ExpiresAt:     replayNow.Add(59*time.Minute + 30*time.Second),
				RecacheTokens: 187_000, WriteTokens: 162_000,
				Warm: model.Some(true), Observed: model.Some(true),
			},
		},
		Limits: model.RateLimits{
			FiveHour: model.Some(model.RateWindow{UsedPct: 8, ResetsAt: replayNow.Add(4*time.Hour + 23*time.Minute + 30*time.Second)}),
			SevenDay: model.Some(model.RateWindow{UsedPct: 88, ResetsAt: replayNow.Add(68*time.Hour + 20*time.Minute)}),
		},
		Usage: model.Some(model.ContextUsage{Pct: 18.7, Used: 187_002, Remain: 812_998, Size: 1_000_000}),
		Activity: model.Some(model.Activity{
			Turns:         1,
			WorkedSeconds: 90,
			StreakStart:   replayNow.Add(-90 * time.Second),
			Samples: []model.Sample{
				{At: replayNow.Add(-10 * time.Minute), Tokens: 4_000},
				{At: replayNow, Tokens: 187_002},
			},
		}),
		Facts: model.Facts{
			Spend: model.Some(model.Spend{
				Today:         model.Some(model.Amount{Text: "375.33", Value: 375.33}),
				Block:         model.Some(model.Amount{Text: "128.68", Value: 128.68}),
				BlockLeftText: "1h53m", BlockLeft: 113 * time.Minute,
				BurnPerHour: model.Some(model.Amount{Text: "41.54", Value: 41.54}),
			}),
			Codex: model.Some(model.CodexLimits{
				Primary: model.Some(model.CodexWindow{UsedPct: 100, WindowMinutes: 10_080, ResetsAt: replayNow.Add(-time.Hour)}),
				Balance: model.Some(68.9),
				SeenAt:  time.Date(2026, 10, 1, 9, 0, 0, 0, replayNow.Location()),
			}),
			Transcript: model.Some(model.Transcript{
				Tools: model.ToolStats{Total: 9, Top: []model.ToolCount{
					{Name: "Bash", Count: 4}, {Name: "Read", Count: 4}, {Name: "Skill", Count: 1},
				}},
				Tokens: model.TokenTotals{
					Input: 6_240, CacheWrite: 150_000, CacheRead: 711_760,
					Output: 5_000, Thinking: 3_500,
				},
				Requests: 5, Prompts: 1,
				ThinkingSeconds: 70,
				Hooks:           11,
				PermissionMode:  "auto",
				Trace:           model.Trace{ObservedBytes: 40_000, LargestObserved: 24_000},
			}),
			Git: model.Some(model.Git{
				Branch: "develop", Changed: 9, Untracked: 9,
				LastCommit:    replayNow.Add(-(time.Hour + 44*time.Minute)),
				DefaultBranch: "develop",
				CommitsToday:  51, FixesToday: 6, AICommitsToday: 38, ConventionalToday: 38,
				TodayAdded: 91_710, TodayDeleted: 1_470, LargestToday: 69_785,
				Streak: 5, LongestStreak: 5, SwitchesToday: 2,
			}),
			Workload: model.Some(model.Workload{
				Issues: 109, Overdue: 39, MostLate: 617, LatestIssue: 85,
				DueWeek: 1, Undated: 69, Stale: 101, Urgent: 4,
				Open: 1, Merged: 100, MergedCapped: true, LeadP50: 5 * time.Minute, Abandoned: 1,
			}),
			Account: model.Some("business.someone@example.com"),
			Machine: model.Machine{
				Load: model.Some(24.5), CPUs: 10,
				FreeMemory: model.Some(5.4 * bytesPerGiB), FreeDisk: model.Some(27.0 * bytesPerGiB),
				Battery:  model.Some(model.Battery{Percent: 100, OnPower: true}),
				Sessions: 9,
			},
		},
		Peers: model.Roster{{
			Key: "peer", Name: "6.3節までの図の改善", Branch: "docs/forms-figures-redraw",
			ContextPct: model.Some(18.4), CostUSD: model.Some(45.96), At: replayNow,
		}},
		Running: 2,
	}
}

func TestComposeReplaysTheProductionScreen(t *testing.T) {
	t.Parallel()
	view := replayView()
	want := []string{
		"🧠 Context: ██░░░░░░░░ 19% 187k/1.00M 残 813k ⏳ ETA 44m",
		": ⚡ Claude 5h ░░░░░ 8% 🔄 4h23m 7d ████░ 88% 🔄 2d20h Pace ×1.5",
		": 🤖 Codex 7d █████ 100% 🔄 済 💳 Bal $68 (10/1)",
		"💰 Cost: Sess $4.41 | Today $375.33 | Blk $128.68 (残 1h53m) | Est $207",
		"🔥 Burn: $41.54/h | ⏰ 5m (API 1m) | Active 1m | Streak 1m",
		"🌿 Git: develop | 📝 9 Files | ❓ 9 New | 📅 Cmt 1h44m (×51 today)",
		"🔧 Work: Turns ×1 | Tools ×9 Bash4/Read4/Skill1 | Hooks ×11",
		"⏰ Due: Overdue ×39 (#85 617d) | Urgent >48h ×4 | Due 7d ×1 | Stale 14d+ ×101 | Undated 69/109 | WIP 1 PR | Lead p50 5m ×100+/14d | Abandoned ×1/14d",
		"🔖 Session: Claideプラグインテストカバレッジ実装",
		"👥 Sessions: Live ×2 | 6.3節までの図の改善 docs/forms-figure… Ctx 18% $45.96",
		"📈 KPI: Focus 100% | Longest 1m | $/Turn 4.41",
		"🚀 Perf: Parallel ×0.30 | Think time 1m | ErrRate 0.0%",
		"📦 Cache: Hit 85% TTL 1h 残 59m | Save 76% | 🧊 Cold 187k | Write 162k",
		"📊 Tokens: Σ In 868k / Out 5k (cached 82%) | Think share 70% | Req ×5 | Last new 2 wr 2k rd 185k → out 201",
		"🤝 Agent: Prompts ×1 | Auto ×9.0/prompt",
		"🧪 Quality: Fix 6/51 today | AI 38/51 today",
		"🧬 Trace: Obs ≈10k (5% ctx) max 6k",
		"🎓 Habits: On develop directly | Switch ×2 today | 1827 L/cmt ×51/day max 69785 +/- 62.4 | Conv 38/51 | Streak 5d (max 5d)",
		"🧭 Env: Fable 5 | Mode high·think | 📂 example-portal | Style Explanatory | Perm auto",
		"💻 System: 🕐 11:06 | CPU 24.5/10c | 🧮 Mem 5.4G | 💾 Disk 27G | 🔌 Bat 100%",
		"🧾 Meta: ⛵ Proc ×9 | 👤 business.someone | v2.1.289",
	}
	if got := render(Compose(&view)); !slices.Equal(got, want) {
		t.Errorf("Compose() =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	// Two alarms blinked: the Codex week at 100 % and the work done on the
	// default branch with uncommitted changes.
	if got, want := alarms(Compose(&view)), []string{"7d █████ 100% 🔄 済", "On develop directly"}; !slices.Equal(got, want) {
		t.Errorf("alarms = %q, want %q", got, want)
	}
}

// The subtle tones of the screen: a pace shown as ×1.5 that is still below
// the danger mark (88 % used at 59.5 % elapsed is ×1.48, written "1.5"), and
// a load of 2.45 CPUs' worth per core.
func TestComposeReplayTones(t *testing.T) {
	t.Parallel()
	view := replayView()
	view.Now = replayNow
	tests := []struct {
		text string
		want model.Tone
	}{
		{"Pace ×1.5", model.ToneCaution},
		{"24.5", model.ToneDanger},
		{"1827 L/cmt", model.ToneCaution},
		{"Perm auto", model.ToneCaution},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			t.Parallel()
			for _, g := range Compose(&view) {
				for _, c := range g.Chips {
					for _, s := range c {
						if s.Text == tt.text {
							if s.Tone != tt.want {
								t.Errorf("tone of %q = %v, want %v", tt.text, s.Tone, tt.want)
							}
							return
						}
					}
				}
			}
			t.Errorf("no span %q on the screen", tt.text)
		})
	}
}

// The identities that made the screen's numbers trustworthy. Each pair of
// sides is read from a different chip of the same render, so a drift between
// two sources shows up as a broken identity, not as a plausible-looking lie.
func TestComposeReplayIdentities(t *testing.T) {
	t.Parallel()
	view := replayView()
	window := view.Session.Context
	usage, _ := view.Usage.Get()
	t.Run("the last request's tokens sum to the context", func(t *testing.T) {
		t.Parallel()
		if got := window.Fresh + window.Written + window.Read; got != usage.Used {
			t.Errorf("new+wr+rd = %v, context used = %v", got, usage.Used)
		}
	})
	t.Run("used and remaining tokens sum to the window", func(t *testing.T) {
		t.Parallel()
		if got := usage.Used + usage.Remain; got != usage.Size {
			t.Errorf("used+remain = %v, size = %v", got, usage.Size)
		}
	})
	w, _ := view.Facts.Workload.Get()
	t.Run("the issue counts sum to the total", func(t *testing.T) {
		t.Parallel()
		if got := w.Overdue + w.DueToday + w.DueWeek + w.Undated; got != w.Issues {
			t.Errorf("overdue+today+week+undated = %d, issues = %d", got, w.Issues)
		}
	})
	tr, _ := view.Facts.Transcript.Get()
	t.Run("the named tools sum to the total", func(t *testing.T) {
		t.Parallel()
		sum := 0
		for _, top := range tr.Tools.Top {
			sum += top.Count
		}
		if sum != tr.Tools.Total {
			t.Errorf("top tools sum to %d, total = %d", sum, tr.Tools.Total)
		}
	})
	git, _ := view.Facts.Git.Get()
	t.Run("no commit habit exceeds the day's commits", func(t *testing.T) {
		t.Parallel()
		for name, n := range map[string]int{"fixes": git.FixesToday, "AI": git.AICommitsToday, "conventional": git.ConventionalToday} {
			if n > git.CommitsToday {
				t.Errorf("%s = %d of %d commits", name, n, git.CommitsToday)
			}
		}
	})
}

// The screen was drawn at 65-66 columns, where Claude Code showed 63 cells
// and cut a 64-cell line to "Est $2…". With the margin of three, no line of
// this screen can reach the cells the host cuts.
func TestComposeReplayLayoutFitsTheHost(t *testing.T) {
	t.Parallel()
	view := replayView()
	const columns = 66
	budget := Budget(columns)
	display := columns - 3 // what the host showed before cutting, measured 2026-10-05
	for _, line := range Layout(Compose(&view), budget) {
		width := line.Indent
		for _, item := range line.Items {
			switch item.Sep {
			case model.SepChip:
				width += SeparatorCells
			case model.SepTight:
				width += TightSeparatorCells
			case model.SepNone:
			}
			width += Cells(item.Chip.Text())
		}
		if width > display {
			t.Errorf("a line of %d cells would be cut by the host: %q", width, draw([]model.Line{line})[0])
		}
	}
}
