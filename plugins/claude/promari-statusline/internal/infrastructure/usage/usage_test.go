package usage_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform/platformtest"
	"promari-statusline/internal/infrastructure/usage"
)

var t0 = time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC)

var errMissing = errors.New("executable file not found")

func amount(text string, value float64) model.Optional[model.Amount] {
	return model.Some(model.Amount{Text: text, Value: value})
}

func TestCCUsage(t *testing.T) {
	t.Parallel()
	const command = "ccusage statusline --refresh-interval 10"
	tests := []struct {
		name string
		out  platformtest.Result
		want model.Spend
		err  error
	}{
		{
			"the full summary, with colours",
			platformtest.Result{Out: "\x1b[32m💰 $1.23 session\x1b[0m / $1,045.67 today / $8.90 block (2h 15m left) | 🔥 $3.21/hr\n"},
			model.Spend{
				Today: amount("1,045.67", 1045.67), Block: amount("8.90", 8.9), BlockLeftText: "2h15m",
				BlockLeft: 2*time.Hour + 15*time.Minute, BurnPerHour: amount("3.21", 3.21),
			},
			nil,
		},
		{
			"minutes only",
			platformtest.Result{Out: "$0.50 today / $0.50 block (45m left)"},
			model.Spend{Today: amount("0.50", 0.5), Block: amount("0.50", 0.5), BlockLeftText: "45m", BlockLeft: 45 * time.Minute},
			nil,
		},
		{
			"hours only",
			platformtest.Result{Out: "$0.50 block (3h left)"},
			model.Spend{Block: amount("0.50", 0.5), BlockLeftText: "3h", BlockLeft: 3 * time.Hour},
			nil,
		},
		{"a block without time left", platformtest.Result{Out: "$2.00 block"}, model.Spend{Block: amount("2.00", 2)}, nil},
		{
			"an amount that is not a number is not an amount",
			platformtest.Result{Out: "$1.2.3 today / $4.00 block"},
			model.Spend{Block: amount("4.00", 4)},
			nil,
		},
		{"a summary with nothing in it", platformtest.Result{Out: "no usage data"}, model.Spend{}, repository.ErrNone},
		{"ccusage is not installed", platformtest.Result{Err: errMissing}, model.Spend{}, errMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Cmds[command] = tt.out
			got, err := usage.CCUsage{Sys: sys}.Spend(context.Background(), []byte(`{"session_id":"s1"}`))
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("Spend() = %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
			if stdin := string(sys.Stdin(command)); stdin != `{"session_id":"s1"}` {
				t.Errorf("ccusage received %q on standard input", stdin)
			}
		})
	}
}

func TestCodex(t *testing.T) {
	t.Parallel()
	const dir = "/h/.codex/sessions/2026/10/01/"
	week := model.Some(model.CodexWindow{UsedPct: 100, WindowMinutes: 10080, ResetsAt: time.Unix(1759550000, 0)})
	tests := []struct {
		name  string
		files map[string]string
		times map[string]time.Time
		want  model.CodexLimits
		err   error
	}{
		{
			"the last line with limits of the newest log wins",
			map[string]string{
				dir + "old.jsonl": `{"rate_limits":{"primary":{"used_percent":1}}}`,
				dir + "new.jsonl": `{"type":"session_meta"}
{"payload":{"info":{"rate_limits":{"primary":{"used_percent":50,"window_minutes":10080}}}}}
not json "rate_limits"
{"payload":[{"x":1},{"rate_limits":{"primary":{"used_percent":100,"window_minutes":10080,"resets_at":1759550000},"secondary":null,"credits":{"balance":"68.5"}}}]}
{"type":"token_count","info":null}
`,
			},
			map[string]time.Time{dir + "old.jsonl": t0.Add(-time.Hour), dir + "new.jsonl": t0},
			model.CodexLimits{Primary: week, Balance: model.Some(68.5), SeenAt: t0},
			nil,
		},
		{
			"a balance written as a number, and a second window",
			map[string]string{dir + "a.jsonl": `{"rate_limits":{"primary":{"used_percent":5,"window_minutes":300},"secondary":{"used_percent":7,"window_minutes":10080},"credits":{"balance":12}}}`},
			nil,
			model.CodexLimits{
				Primary:   model.Some(model.CodexWindow{UsedPct: 5, WindowMinutes: 300}),
				Secondary: model.Some(model.CodexWindow{UsedPct: 7, WindowMinutes: 10080}),
				Balance:   model.Some(12.0), SeenAt: t0,
			},
			nil,
		},
		{
			"a balance that is not a number, and a window without a percentage",
			map[string]string{dir + "a.jsonl": `{"rate_limits":{"primary":{"window_minutes":300},"credits":{"balance":"unlimited"}}}`},
			nil,
			model.CodexLimits{SeenAt: t0},
			nil,
		},
		{
			"empty limits are skipped in favour of nested ones, searched by name",
			map[string]string{dir + "a.jsonl": `{"rate_limits":{},"b":{"rate_limits":{"primary":{"used_percent":2}}},"a":{"rate_limits":{"primary":{"used_percent":1}}}}`},
			nil,
			model.CodexLimits{Primary: model.Some(model.CodexWindow{UsedPct: 1}), SeenAt: t0},
			nil,
		},
		{"a log without limits", map[string]string{dir + "a.jsonl": `{"type":"session_meta"}` + "\n"}, nil, model.CodexLimits{}, repository.ErrNone},
		{"limits that are not an object", map[string]string{dir + "a.jsonl": `{"rate_limits":"none"}`}, nil, model.CodexLimits{}, repository.ErrNone},
		{"no codex logs", nil, nil, model.CodexLimits{}, repository.ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			for name, content := range tt.files {
				sys.Files[name] = []byte(content)
			}
			sys.Times = tt.times
			got, err := usage.Codex{Sys: sys}.Codex(context.Background())
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("Codex() = %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

// The board is read by other tools, so the test pins the bytes: a member that
// is renamed here is a member another tool no longer finds.
func TestBoard(t *testing.T) {
	t.Parallel()
	const (
		claudePath = "/h/.cache/claude-rate-limits.json"
		codexPath  = "/h/.cache/codex-rate-statusline.json"
	)
	reset := time.Unix(1790985000, 0)
	at := time.UnixMilli(1790971649050)

	t.Run("the Claude windows", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			limits model.RateLimits
			want   string
		}{
			{
				"both windows",
				model.RateLimits{
					FiveHour: model.Some(model.RateWindow{UsedPct: 29, ResetsAt: reset}),
					SevenDay: model.Some(model.RateWindow{UsedPct: 80.5, ResetsAt: reset.Add(time.Hour)}),
				},
				`{"ts":1790971649.05,"rl":{"five_hour":{"used_percentage":29,"resets_at":1790985000},"seven_day":{"used_percentage":80.5,"resets_at":1790988600}}}`,
			},
			{
				"a window without a reset time, and one that is not known",
				model.RateLimits{SevenDay: model.Some(model.RateWindow{UsedPct: 0})},
				`{"ts":1790971649.05,"rl":{"seven_day":{"used_percentage":0}}}`,
			},
			{
				"the spend limit is not a window its readers know",
				model.RateLimits{Spend: model.Some(model.RateWindow{UsedPct: 10})},
				`{"ts":1790971649.05,"rl":{}}`,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				sys := platformtest.New(t0)
				sys.Env["XDG_CACHE_HOME"] = "/elsewhere" // the board does not move with the plugin's cache
				if err := (usage.Board{Sys: sys}).PostClaude(tt.limits, at); err != nil {
					t.Fatal(err)
				}
				if got, _ := sys.File(claudePath); got != tt.want {
					t.Errorf("posted\n  %s\nwant\n  %s", got, tt.want)
				}
			})
		}
	})

	t.Run("the Codex windows", func(t *testing.T) {
		t.Parallel()
		limits := model.CodexLimits{
			Primary: model.Some(model.CodexWindow{UsedPct: 100, WindowMinutes: 10080, ResetsAt: reset}),
			SeenAt:  time.UnixMilli(1790827230990),
		}
		const want = `{"rl":{"primary":{"used_percent":100,"window_minutes":10080,"resets_at":1790985000}},"mtime":1790827230.99}`
		sys := platformtest.New(t0)
		board := usage.Board{Sys: sys}
		if err := board.PostCodex(limits, t0); err != nil {
			t.Fatal(err)
		}
		if got, _ := sys.File(codexPath); got != want {
			t.Fatalf("posted\n  %s\nwant\n  %s", got, want)
		}

		// Its readers judge the file by its modification time, so an unchanged
		// file is written again after a minute, and not before.
		written := func() time.Time {
			at, err := sys.ModTime(codexPath)
			if err != nil {
				t.Fatal(err)
			}
			return at
		}
		sys.T = t0.Add(59 * time.Second)
		if err := board.PostCodex(limits, sys.T); err != nil || !written().Equal(t0) {
			t.Errorf("an unchanged file was written again after 59s: %v, %v", written(), err)
		}
		sys.T = t0.Add(time.Minute)
		if err := board.PostCodex(limits, sys.T); err != nil || !written().Equal(sys.T) {
			t.Errorf("an unchanged file was not refreshed after a minute: %v, %v", written(), err)
		}
		// A change is posted at once.
		limits.Secondary = model.Some(model.CodexWindow{UsedPct: 12.5})
		sys.T = sys.T.Add(time.Second)
		if err := board.PostCodex(limits, sys.T); err != nil || !written().Equal(sys.T) {
			t.Errorf("a change was not posted: %v, %v", written(), err)
		}
		if got, _ := sys.File(codexPath); !strings.Contains(got, `"secondary":{"used_percent":12.5}`) {
			t.Errorf("posted %s", got)
		}
	})

	t.Run("a board that cannot be written is an error", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		sys.ReadOnly = true
		board := usage.Board{Sys: sys}
		if err := board.PostClaude(model.RateLimits{}, t0); err == nil {
			t.Error("PostClaude on a read-only machine succeeded")
		}
		if err := board.PostCodex(model.CodexLimits{}, t0); err == nil {
			t.Error("PostCodex on a read-only machine succeeded")
		}
	})
}
