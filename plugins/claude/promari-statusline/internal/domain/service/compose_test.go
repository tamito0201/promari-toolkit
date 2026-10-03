package service

import (
	"slices"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

var now = time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC)

// render writes groups as "Title: chip | chip", one string per group.
func render(groups []model.Group) []string {
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		chips := make([]string, 0, len(g.Chips))
		for _, c := range g.Chips {
			chips = append(chips, c.Text())
		}
		out = append(out, g.Title+": "+strings.Join(chips, " | "))
	}
	return out
}

// alarms returns the text of every alarm span, group by group.
func alarms(groups []model.Group) []string {
	var out []string
	for _, g := range groups {
		for _, c := range g.Chips {
			var b strings.Builder
			for _, s := range c {
				if s.Alarm {
					b.WriteString(s.Text)
				}
			}
			if b.Len() > 0 {
				out = append(out, b.String())
			}
		}
	}
	return out
}

func usage(pct float64) model.Optional[model.ContextUsage] {
	return model.Some(model.ContextUsage{Pct: pct, Used: pct * 2000, Remain: 200_000 - pct*2000, Size: 200_000})
}

func window(pct float64, resetsIn time.Duration) model.Optional[model.RateWindow] {
	return model.Some(model.RateWindow{UsedPct: pct, ResetsAt: now.Add(resetsIn)})
}

func amount(text string, value float64) model.Optional[model.Amount] {
	return model.Some(model.Amount{Text: text, Value: value})
}

func TestCompose(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		view View
		want []string
	}{
		{
			"nothing reported: the clock and an unknown model",
			View{},
			[]string{env, system},
		},
		{
			"a session that has not answered yet",
			View{Session: model.Session{Reported: true, Model: "Opus"}},
			[]string{"🧠 Context: 初回応答待ち", "🧭 Env: Opus", system},
		},
		{
			"context with an estimate",
			View{
				Usage:    usage(42),
				Activity: model.Some(model.Activity{Samples: []model.Sample{{At: now.Add(-time.Minute), Tokens: 83_000}, {At: now, Tokens: 84_000}}, StreakStart: now}),
			},
			[]string{"🧠 Context: ████░░░░░░ 42% 84k/200k 残 116k ⏳ ETA 1h56m", "🔥 Burn: Active 0m | Streak 0m", env, system},
		},
		{
			"an incident",
			View{Facts: model.Facts{Incident: model.Some(model.Incident{Indicator: "minor", Description: "Elevated error rates on the Messages API for some requests"})}},
			[]string{"🚨 Alert: 🌐 API minor: Elevated error rates on the Me", env, system},
		},
		{
			"an operational status page shows nothing",
			View{Facts: model.Facts{Incident: model.Some(model.Incident{Indicator: "none"})}},
			[]string{env, system},
		},
		{
			"forecasts",
			View{Forecasts: []model.Forecast{{Label: "5h", In: 50 * time.Minute}}},
			[]string{"📉 Forecast: 5h 枯渇まで 50m (reset前)", env, system},
		},
		{
			"the rate windows, with pace on the one running ahead",
			View{Limits: model.RateLimits{
				FiveHour: window(1, 4*time.Hour+40*time.Minute),
				SevenDay: window(73, 4*24*time.Hour+7*time.Hour),
				Spend:    model.Some(model.RateWindow{UsedPct: 12}),
			}},
			[]string{": ⚡ Claude 5h ░░░░░ 1% 🔄 4h40m 7d ████░ 73% 🔄 4d7h Pace ×1.9 Spend █░░░░ 12%", env, system},
		},
		{
			"remembered rate limits older than six hours show their date",
			View{Limits: model.RateLimits{FiveHour: window(10, time.Hour)}, LimitsSeen: now.Add(-7 * time.Hour)},
			[]string{": ⚡ Claude 5h ░░░░░ 10% 🔄 1h00m (10/2)", env, system},
		},
		{
			"remembered rate limits from just now show no date",
			View{Limits: model.RateLimits{FiveHour: window(10, time.Hour)}, LimitsSeen: now.Add(-time.Hour)},
			[]string{": ⚡ Claude 5h ░░░░░ 10% 🔄 1h00m", env, system},
		},
		{
			"codex: a week window, a short second window, a balance and a date",
			View{Facts: model.Facts{Codex: model.Some(model.CodexLimits{
				Primary:   model.Some(model.CodexWindow{UsedPct: 100, WindowMinutes: 10_080, ResetsAt: now.Add(24*time.Hour + 19*time.Minute)}),
				Secondary: model.Some(model.CodexWindow{UsedPct: 5, WindowMinutes: 30}),
				Balance:   model.Some(68.9),
				SeenAt:    now.Add(-48 * time.Hour),
			})}},
			[]string{": 🤖 Codex 7d █████ 100% 🔄 24h19m 2:30m ░░░░░ 5% 💳 Bal $68 (10/1)", env, system},
		},
		{
			"codex: a five-hour window",
			View{Facts: model.Facts{Codex: model.Some(model.CodexLimits{Primary: model.Some(model.CodexWindow{UsedPct: 5, WindowMinutes: 300}), SeenAt: now})}},
			[]string{": 🤖 Codex 5h ░░░░░ 5%", env, system},
		},
		{
			"codex without a window or a balance says nothing",
			View{Facts: model.Facts{Codex: model.Some(model.CodexLimits{SeenAt: now})}},
			[]string{env, system},
		},
		{
			"cost, burn, KPI and perf from a session with spending",
			View{
				Session: model.Session{
					Cost:    model.Cost{TotalUSD: model.Some(12.5), Wall: 2 * time.Hour, API: 3 * time.Hour, LinesAdded: 1200, LinesRemoved: 30},
					Context: model.ContextWindow{TotalInput: 5_400_000, TotalOutput: 120_000},
				},
				Activity: model.Some(model.Activity{Turns: 25, WorkedSeconds: 3600, IdledSeconds: 1200, StreakStart: now.Add(-20 * time.Minute)}),
				Facts: model.Facts{Spend: model.Some(model.Spend{
					Today: amount("45.67", 45.67), Block: amount("8.90", 8.9), BlockLeftText: "2h15m", BlockLeft: 135 * time.Minute, BurnPerHour: amount("3.21", 3.21),
				})},
			},
			[]string{
				"💰 Cost: Sess $12.50 | Today $45.67 | Blk $8.90 (残 2h15m) | Est $16",
				"🔥 Burn: $3.21/h | ⏰ 2h00m (API 3h00m) | Active 1h00m | Streak 20m | Idle 20m",
				"📈 KPI: Lines +1200-30 | Net +1,170 | Focus 75% | Lines/h 1,200 | Longest 20m | $/Line 0.010 | $/Turn 0.50",
				"🚀 Perf: Parallel ×1.50",
				"🔧 Work: Turns ×25",
				env, system,
			},
		},
		{
			"a session cost of zero is shown; ratios over it are not",
			View{Session: model.Session{Cost: model.Cost{TotalUSD: model.Some(0.0), LinesAdded: 10}}},
			[]string{"💰 Cost: Sess $0.00", "📈 KPI: Lines +10-0 | Net +10", env, system},
		},
		{
			"ratios wait for a denominator that means something",
			View{
				Session:  model.Session{Cost: model.Cost{LinesAdded: 500}},
				Activity: model.Some(model.Activity{WorkedSeconds: 59, StreakStart: now}),
			},
			[]string{"🔥 Burn: Active 0m | Streak 0m", "📈 KPI: Lines +500-0 | Net +500", env, system},
		},
		{
			"a block without time left has no estimate; wall time without API time",
			View{
				Session: model.Session{Cost: model.Cost{Wall: 10 * time.Minute}},
				Facts:   model.Facts{Spend: model.Some(model.Spend{Block: amount("1,234.50", 1234.5)})},
			},
			[]string{"💰 Cost: Blk $1,234.50", "🔥 Burn: ⏰ 10m", env, system},
		},
		{
			"the prompt cache",
			View{Session: model.Session{Cache: model.PromptCache{
				HitRatio: model.Some(0.93), Misses: 2, LastMissCause: "ttl", ExpiresAt: now.Add(55 * time.Minute), RecacheTokens: 84_000,
			}}},
			[]string{"📦 Cache: Hit 93% 2 Miss (ttl) 残 55m | Save 84% | 🧊 Cold 84k", env, system},
		},
		{
			"a cache without misses or expiry",
			View{Session: model.Session{Cache: model.PromptCache{HitRatio: model.Some(1.0), ExpiresAt: now.Add(-time.Minute)}}},
			[]string{"📦 Cache: Hit 100% | Save 90%", env, system},
		},
		{
			"tokens: the surcharge and compactions",
			View{Session: model.Session{Over200k: true}, Activity: model.Some(model.Activity{Compactions: 2, StreakStart: now})},
			[]string{"🧠 Context: compact ×2", "🔥 Burn: Active 0m | Streak 0m", "📊 Tokens: 🚧 200k超割増", env, system},
		},
		{
			"work: to-dos and tools with errors",
			View{Facts: model.Facts{
				Todos:      model.Some(model.Todos{Done: 3, Total: 7, Doing: "Write the tests for the layout engine"}),
				Transcript: model.Some(model.Transcript{Tools: model.ToolStats{Total: 200, Errors: 5, Top: []model.ToolCount{{Name: "Bash", Count: 120}, {Name: "Read", Count: 50}, {Name: "Edit", Count: 30}}}}),
			}},
			[]string{"🚀 Perf: ErrRate 2.5%", "🔧 Work: ✅ Todo 3/7 (Write the tests for…) | Tools ×200 Bash120/Read50/Edit30 ❌ Err 5", env, system},
		},
		{
			"work: to-dos without one in progress, tools without errors",
			View{Facts: model.Facts{
				Todos:      model.Some(model.Todos{Done: 2, Total: 2}),
				Transcript: model.Some(model.Transcript{Tools: model.ToolStats{Total: 3, Top: []model.ToolCount{{Name: "Read", Count: 3}}}}),
			}},
			[]string{"🚀 Perf: ErrRate 0.0%", "🔧 Work: ✅ Todo 2/2 | Tools ×3 Read3", env, system},
		},
		{
			"git with a failing pull request",
			View{Facts: model.Facts{
				Git:  model.Some(model.Git{Branch: "develop", Changed: 2, Ahead: 1, Behind: 16, Stashes: 3, LastCommit: now.Add(-21*time.Hour - 28*time.Minute)}),
				Pull: model.Some(model.PullRequest{Number: 2996, Passed: 10, Failed: 2, Pending: 1, Review: model.ReviewChangesRequested}),
			}},
			[]string{"🌿 Git: develop | 📝 2 Files | 🔼 1 Ahead | 🔽 16 Behind | 📚 3 Stash | 📅 Cmt 21h28m", "🔀 PR: #2996 CI ❌ 2 changes", env, system},
		},
		{
			"git: a clean branch with a pending, unreviewed pull request",
			View{Facts: model.Facts{
				Git:  model.Some(model.Git{Branch: "main"}),
				Pull: model.Some(model.PullRequest{Number: 7, Passed: 3, Pending: 1, Review: model.ReviewRequired}),
			}},
			[]string{"🌿 Git: main", "🔀 PR: #7 CI ⏳ 1 review", env, system},
		},
		{
			"git: an approved pull request with green checks, and one without checks",
			View{Facts: model.Facts{
				Git:  model.Some(model.Git{Branch: "main"}),
				Pull: model.Some(model.PullRequest{Number: 8, Passed: 3, Review: model.ReviewApproved}),
			}},
			[]string{"🌿 Git: main", "🔀 PR: #8 CI ✅ 3 approved", env, system},
		},
		{
			"git: a pull request with nothing to say but its number",
			View{Facts: model.Facts{Git: model.Some(model.Git{Branch: "main"}), Pull: model.Some(model.PullRequest{Number: 9})}},
			[]string{"🌿 Git: main", "🔀 PR: #9", env, system},
		},
		{
			"no branch, no git group",
			View{Facts: model.Facts{Git: model.Some(model.Git{}), Pull: model.Some(model.PullRequest{Number: 9})}},
			[]string{env, system},
		},
		{
			"the session's name and what it runs with",
			View{Session: model.Session{
				Name: "A very long session name that does not fit into forty characters", Model: "Opus 5.5", Effort: "high", Thinking: true, Fast: true,
				Repo: "promari", Style: "Explanatory", Version: "2.1.34",
			}},
			[]string{
				"🔖 Session: A very long session name that does not…",
				"🧭 Env: Opus 5.5 | Mode high·think·fast | 📂 promari | Style Explanatory",
				system,
				"🧾 Meta: v2.1.34",
			},
		},
		{
			"the machine",
			View{Facts: model.Facts{Machine: model.Machine{
				TerminalStart: model.Some(now.Add(-3 * time.Hour)), Load: model.Some(3.94), CPUs: 10,
				FreeMemory: model.Some(4.8 * bytesPerGiB), FreeDisk: model.Some(80.4 * bytesPerGiB),
				Battery: model.Some(model.Battery{Percent: 100, OnPower: true}), Sessions: 6,
			}}},
			[]string{env, "💻 System: 🕐 04:09 | Term up 3h00m | CPU 3.9/10c | 🧮 Mem 4.8G | 💾 Disk 80G | 🔌 Bat 100%", "🧾 Meta: ⛵ Proc ×6"},
		},
		{
			"meta: the account, and an update only when it is newer",
			View{
				Session: model.Session{Version: "2.1.34"},
				Facts:   model.Facts{Account: model.Some("someone@example.com"), Latest: model.Some("2.1.287"), Machine: model.Machine{Sessions: 1}},
			},
			[]string{env, system, "🧾 Meta: 👤 someone | v2.1.34 | 🆙 Update v2.1.287"},
		},
		{
			"meta: the installed version is the latest",
			View{Session: model.Session{Version: "2.1.287"}, Facts: model.Facts{Latest: model.Some("2.1.287")}},
			[]string{env, system, "🧾 Meta: v2.1.287"},
		},
		{
			"music, paused",
			View{Facts: model.Facts{Track: model.Some(model.Track{Title: "Take Five", Artist: "The Dave Brubeck Quartet", Paused: true})}},
			[]string{env, system, "🎵 Music: Take Five — The Dave Brubeck Quartet (Paused)"},
		},
		{
			"music without an artist, cut at forty characters",
			View{Facts: model.Facts{Track: model.Some(model.Track{Title: "Symphony No. 9 in D minor, Op. 125 \"Choral\": IV. Presto"})}},
			[]string{env, system, "🎵 Music: Symphony No. 9 in D minor, Op. 125 \"Cho…"},
		},
		{
			"a track without a title is no music",
			View{Facts: model.Facts{Track: model.Some(model.Track{Artist: "Someone"})}},
			[]string{env, system},
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

func TestComposeAlarms(t *testing.T) {
	t.Parallel()
	lowBattery := model.Facts{Machine: model.Machine{Battery: model.Some(model.Battery{Percent: 9})}}
	tests := []struct {
		name string
		view View
		want []string
	}{
		{"usage below ninety percent is no alarm", View{Usage: usage(89.9)}, nil},
		{"a context at ninety percent", View{Usage: usage(90)}, []string{"█████████░ 90% 180k/200k 残 20k"}},
		{
			"only the rate window over the mark (with its pace), not its neighbours or the header",
			View{Limits: model.RateLimits{FiveHour: window(95, 4*time.Hour), SevenDay: window(10, 150*time.Hour)}},
			[]string{"5h █████ 95% 🔄 4h00m Pace ×4.8"},
		},
		{
			"a codex window over the mark",
			View{Facts: model.Facts{Codex: model.Some(model.CodexLimits{Primary: model.Some(model.CodexWindow{UsedPct: 100, WindowMinutes: 10_080}), Balance: model.Some(5.0), SeenAt: now})}},
			[]string{"7d █████ 100%"},
		},
		{"a major incident", View{Facts: model.Facts{Incident: model.Some(model.Incident{Indicator: "major", Description: "Outage"})}}, []string{"🌐 API major: Outage"}},
		{"a minor incident", View{Facts: model.Facts{Incident: model.Some(model.Incident{Indicator: "minor", Description: "Slow"})}}, nil},
		{"a forecast", View{Forecasts: []model.Forecast{{Label: "7d", In: time.Hour}}}, []string{"7d 枯渇まで 1h00m (reset前)"}},
		{"a low battery", View{Facts: lowBattery}, []string{"🔋 Bat 9%"}},
		{
			"a low battery that is charging",
			View{Facts: model.Facts{Machine: model.Machine{Battery: model.Some(model.Battery{Percent: 9, OnPower: true})}}},
			nil,
		},
		{
			"the demo makes every usage chip and the battery an alarm",
			View{AlarmAll: true, Usage: usage(1), Facts: model.Facts{Machine: model.Machine{Battery: model.Some(model.Battery{Percent: 80})}}},
			[]string{"░░░░░░░░░░ 1% 2k/200k 残 198k", "🔋 Bat 80%"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			view := tt.view
			view.Now = now
			if got := alarms(Compose(&view)); !slices.Equal(got, tt.want) {
				t.Errorf("alarms = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComposeTones(t *testing.T) {
	t.Parallel()
	tone := func(view View, title, text string) model.Tone {
		view.Now = now
		for _, g := range Compose(&view) {
			if g.Title != title {
				continue
			}
			for _, c := range g.Chips {
				for _, s := range c {
					if s.Text == text {
						return s.Tone
					}
				}
			}
		}
		t.Errorf("no span %q in group %q", text, title)
		return model.TonePlain
	}
	battery := func(pct int) View {
		return View{Facts: model.Facts{Machine: model.Machine{Battery: model.Some(model.Battery{Percent: pct, OnPower: true})}}}
	}
	load := func(l float64) View {
		return View{Facts: model.Facts{Machine: model.Machine{Load: model.Some(l), CPUs: 10}}}
	}
	errors := func(n int) View {
		return View{Facts: model.Facts{Transcript: model.Some(model.Transcript{Tools: model.ToolStats{Total: 100, Errors: n}})}}
	}
	pace := func(pct float64) View {
		// Half of the five-hour window has passed.
		return View{Limits: model.RateLimits{FiveHour: window(pct, 150*time.Minute)}}
	}
	tests := []struct {
		name  string
		view  View
		title string
		text  string
		want  model.Tone
	}{
		{"battery at fifty is good", battery(50), "💻 System", "50%", model.ToneGood},
		{"battery at forty-nine is a caution", battery(49), "💻 System", "49%", model.ToneCaution},
		{"battery at twenty is a caution", battery(20), "💻 System", "20%", model.ToneCaution},
		{"battery at nineteen is a danger", battery(19), "💻 System", "19%", model.ToneDanger},
		{"a load of 69 % of the CPUs is good", load(6.9), "💻 System", "6.9", model.ToneGood},
		{"a load of 70 % of the CPUs is a caution", load(7), "💻 System", "7.0", model.ToneCaution},
		{"a load of all CPUs is a danger", load(10), "💻 System", "10.0", model.ToneDanger},
		{"an error rate under two percent is good", errors(1), "🚀 Perf", "ErrRate 1.0%", model.ToneGood},
		{"an error rate of two percent is a caution", errors(2), "🚀 Perf", "ErrRate 2.0%", model.ToneCaution},
		{"an error rate of five percent is a danger", errors(5), "🚀 Perf", "ErrRate 5.0%", model.ToneDanger},
		{"a pace of 1.05 is a caution", pace(52.5), "", "Pace ×1.1", model.ToneCaution},
		{"a pace of 1.5 is a danger", pace(75), "", "Pace ×1.5", model.ToneDanger},
		{"a minor incident is a caution", View{Facts: model.Facts{Incident: model.Some(model.Incident{Indicator: "minor"})}}, "🚨 Alert", "🌐 API minor: ", model.ToneCaution},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tone(tt.view, tt.title, tt.text); got != tt.want {
				t.Errorf("tone of %q = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
	t.Run("a pace just under 1.05 is not shown", func(t *testing.T) {
		t.Parallel()
		view := pace(52)
		view.Now = now
		if got := render(Compose(&view))[0]; strings.Contains(got, "Pace") {
			t.Errorf("a pace of 1.04 is shown: %s", got)
		}
	})
}
