package statusline_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"promari-statusline/internal/application/usecase"
	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/interfaces/statusline"
)

// The JSON below is indented as JSON is, not as Go is.
// editorconfig-checker-disable
const full = `{
  "session_id": "3f2a", "prompt_id": "p9", "session_name": "refactor", "transcript_path": "/t.jsonl",
  "version": "2.1.34", "fast_mode": true, "exceeds_200k_tokens": true, "cwd": "/fallback",
  "model": {"id": "claude-opus-5-5", "display_name": "Opus 5.5"},
  "effort": {"level": "high"}, "thinking": {"enabled": true}, "output_style": {"name": "Explanatory"},
  "workspace": {"current_dir": "/work", "repo": {"name": "promari"}},
  "cost": {"total_cost_usd": 0, "total_duration_ms": 600000, "total_api_duration_ms": 300000.5, "total_lines_added": 10, "total_lines_removed": 2},
  "context_window": {
    "context_window_size": 200000, "used_percentage": 42, "total_input_tokens": 5400000, "total_output_tokens": 120000,
    "current_usage": {"input_tokens": 4000, "cache_creation_input_tokens": 10000, "cache_read_input_tokens": 70000}
  },
  "rate_limits": {
    "five_hour": {"used_percentage": 0, "resets_at": 1759464540},
    "seven_day": {"used_percentage": 73},
    "spend_limit": {"resets_at": 1759464540}
  },
  "prompt_cache": {"hit_ratio": 0.93, "misses": 2, "last_miss_cause": "ttl", "expires_at": 1759464540.5, "recache_tokens_if_cold": 84000}
}`

// editorconfig-checker-enable

func TestDecode(t *testing.T) {
	t.Parallel()
	t.Run("every field Claude Code sends", func(t *testing.T) {
		t.Parallel()
		got := statusline.Decode([]byte(full))
		want := model.Session{
			Reported: true, ID: "3f2a", PromptID: "p9", Name: "refactor", TranscriptPath: "/t.jsonl", Dir: "/work", Repo: "promari",
			Version: "2.1.34", Model: "Opus 5.5", Effort: "high", Thinking: true, Fast: true, Style: "Explanatory", Over200k: true,
			Cost: model.Cost{
				TotalUSD: model.Some(0.0), Wall: 10 * time.Minute, API: 5*time.Minute + 500*time.Microsecond, LinesAdded: 10, LinesRemoved: 2,
			},
			Context: model.ContextWindow{Size: 200_000, UsedPct: model.Some(42.0), Current: 84_000, TotalInput: 5_400_000, TotalOutput: 120_000},
			Limits: model.RateLimits{
				// Zero percent used is a known window; a window without a percentage is none.
				FiveHour: model.Some(model.RateWindow{UsedPct: 0, ResetsAt: time.Unix(1759464540, 0)}),
				SevenDay: model.Some(model.RateWindow{UsedPct: 73}),
			},
			Cache: model.PromptCache{
				HitRatio: model.Some(0.93), Misses: 2, LastMissCause: "ttl", ExpiresAt: time.Unix(1759464540, 500_000_000), RecacheTokens: 84_000,
			},
		}
		// The times are compared by instant; everything else must be identical.
		if !got.Limits.FiveHour.Or(model.RateWindow{}).ResetsAt.Equal(time.Unix(1759464540, 0)) || !got.Cache.ExpiresAt.Equal(want.Cache.ExpiresAt) {
			t.Errorf("times = %v, %v", got.Limits.FiveHour, got.Cache.ExpiresAt)
		}
		got.Limits, want.Limits = stripTimes(got.Limits), stripTimes(want.Limits)
		got.Cache.ExpiresAt, want.Cache.ExpiresAt = time.Time{}, time.Time{}
		if got != want {
			t.Errorf("Decode() =\n  %+v\nwant\n  %+v", got, want)
		}
	})

	tests := []struct {
		name  string
		raw   string
		check func(*testing.T, model.Session)
	}{
		{"nothing", ``, unreported},
		{"an empty object", `{}`, unreported},
		{"input that is not JSON", `{"model":`, unreported},
		{"input that is not an object", `[1,2]`, unreported},
		{
			"the first render of a session: a model and a directory, nothing else",
			`{"model":{"display_name":"Opus"},"cwd":"/work"}`,
			func(t *testing.T, s model.Session) {
				t.Helper()
				reported(t, s, true)
				if s.Model != "Opus" || s.Dir != "/work" || s.Cost.TotalUSD.Present() || s.Context.UsedPct.Present() || !s.Limits.Empty() || s.Cache.HitRatio.Present() {
					t.Errorf("Decode() = %+v", s)
				}
			},
		},
		{
			"a member that changed its type is lost alone",
			`{"model":"opus","version":"2.1.34","cost":{"total_cost_usd":"1.5","total_lines_added":10},"context_window":"big","fast_mode":"yes","rate_limits":{"five_hour":"soon","seven_day":{"used_percentage":5}}}`,
			func(t *testing.T, s model.Session) {
				t.Helper()
				if s.Model != "" || s.Version != "2.1.34" || s.Cost.TotalUSD.Present() || s.Cost.LinesAdded != 10 || s.Fast {
					t.Errorf("Decode() = %+v", s)
				}
				if s.Limits.FiveHour.Present() || s.Limits.SevenDay.Or(model.RateWindow{}).UsedPct != 5 {
					t.Errorf("limits = %+v", s.Limits)
				}
			},
		},
		{
			"null is the same as absent",
			`{"session_id":"s1","cost":{"total_cost_usd":null},"context_window":{"used_percentage":null,"context_window_size":200000},"rate_limits":null}`,
			func(t *testing.T, s model.Session) {
				t.Helper()
				if s.Cost.TotalUSD.Present() || s.Context.UsedPct.Present() || !s.Limits.Empty() {
					t.Errorf("Decode() = %+v", s)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.check(t, statusline.Decode([]byte(tt.raw)))
		})
	}
}

// unreported is the check of input that carries no report at all.
func unreported(t *testing.T, s model.Session) {
	t.Helper()
	reported(t, s, false)
}

func reported(t *testing.T, s model.Session, want bool) {
	t.Helper()
	if s.Reported != want {
		t.Errorf("Reported = %v, want %v", s.Reported, want)
	}
}

func stripTimes(l model.RateLimits) model.RateLimits {
	if w, ok := l.FiveHour.Get(); ok {
		w.ResetsAt = time.Time{}
		l.FiveHour = model.Some(w)
	}
	return l
}

const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	band   = "\x1b[48;5;196m\x1b[38;5;231m"
	muted  = "\x1b[38;5;245m"
	good   = "\x1b[38;5;114m"
	accent = "\x1b[38;5;141m"
)

func TestPresent(t *testing.T) {
	t.Parallel()
	even := time.Unix(1759464540, 0)
	odd := even.Add(time.Second)
	header := model.Chip{{Text: "🧠 Context", Tone: model.ToneAccent, Bold: true}}
	usage := model.Chip{{Text: "████", Tone: model.ToneGood}, {Text: ""}, {Text: " "}, {Text: "42%", Tone: model.ToneGood}}
	alarm := model.Chip{
		{Text: "⚡ Claude", Tone: model.ToneBrand, Bold: true},
		{Text: " "},
		{Text: "5h", Tone: model.ToneMuted, Alarm: true},
		{Text: " ", Alarm: true},
		{Text: "95%", Tone: model.ToneDanger, Alarm: true},
		{Text: " "},
		{Text: "7d", Tone: model.ToneMuted},
	}
	tests := []struct {
		name  string
		lines []model.Line
		at    time.Time
		want  string
	}{
		{"no lines", nil, even, ""},
		{
			"colours, bold, plain text and both separators",
			[]model.Line{
				{Items: []model.Item{{Chip: header}, {Sep: model.SepChip, Chip: usage}, {Sep: model.SepGroup, Chip: model.Chip{{Text: "plain"}}}}},
				{Indent: 3, Items: []model.Item{{Chip: model.Chip{{Text: "Test", Tone: model.ToneAccent}}}}},
			},
			even,
			accent + bold + "🧠 Context" + reset + muted + " │ " + reset + good + "████" + reset + " " + good + "42%" + reset +
				muted + " ┃ " + reset + "plain" + "\n" + "   " + accent + "Test" + reset,
		},
		{
			"the chips of a packed group stand closer",
			[]model.Line{{Items: []model.Item{{Chip: header}, {Sep: model.SepTight, Chip: model.Chip{{Text: "a"}}}, {Sep: model.SepTight, Chip: model.Chip{{Text: "b"}}}}}},
			even,
			accent + bold + "🧠 Context" + reset + muted + "│" + reset + "a" + muted + "│" + reset + "b",
		},
		{
			"an alarm on an even second keeps its colours",
			[]model.Line{{Items: []model.Item{{Chip: alarm}}}},
			even,
			"\x1b[38;5;208m" + bold + "⚡ Claude" + reset + " " + muted + "5h" + reset + " " + "\x1b[38;5;203m" + "95%" + reset + " " + muted + "7d" + reset,
		},
		{
			"an alarm on an odd second is one red band; its neighbours keep their colours",
			[]model.Line{{Items: []model.Item{{Chip: alarm}}}},
			odd,
			"\x1b[38;5;208m" + bold + "⚡ Claude" + reset + " " + band + bold + "5h 95%" + reset + " " + muted + "7d" + reset,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := statusline.Present(tt.lines, tt.at); got != tt.want {
				t.Errorf("Present() =\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
	t.Run("every tone has a colour of its own, except plain", func(t *testing.T) {
		t.Parallel()
		seen := map[string]model.Tone{}
		for tone := model.TonePlain; tone <= model.ToneBrand; tone++ {
			got := statusline.Present([]model.Line{{Items: []model.Item{{Chip: model.Chip{{Text: "x", Tone: tone}}}}}}, even)
			if tone == model.TonePlain {
				if got != "x" {
					t.Errorf("plain text is styled: %q", got)
				}
				continue
			}
			if other, dup := seen[got]; dup || !strings.HasSuffix(got, "x"+reset) {
				t.Errorf("tone %d renders as %q (same as tone %d: %v)", tone, got, other, dup)
			}
			seen[got] = tone
		}
		if got := statusline.Present([]model.Line{{Items: []model.Item{{Chip: model.Chip{{Text: "x", Tone: model.Tone(200)}}}}}}, even); got != "x" {
			t.Errorf("an unknown tone is styled: %q", got)
		}
	})
}

// renderer records the request and answers with fixed lines.
type renderer struct {
	got usecase.RenderRequest
}

func (r *renderer) Execute(_ context.Context, req usecase.RenderRequest) usecase.Rendered {
	r.got = req
	return usecase.Rendered{
		Lines: []model.Line{{Items: []model.Item{{Chip: model.Chip{{Text: req.Session.Model}}}}}},
		At:    time.Unix(1759464540, 0),
	}
}

var errClosed = errors.New("closed pipe")

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errClosed }

func TestHandle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		in       io.Reader
		want     string
		reported bool
	}{
		{"a session report", strings.NewReader(`{"model":{"display_name":"Opus"}}`), "Opus\n", true},
		{"no input still gives a line", strings.NewReader(""), "\n", false},
		{"input that cannot be read still gives a line", iotest.ErrReader(errClosed), "\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &renderer{}
			var out bytes.Buffer
			if err := (statusline.Handler{Render: r}).Handle(context.Background(), tt.in, &out); err != nil {
				t.Fatal(err)
			}
			if out.String() != tt.want || r.got.Session.Reported != tt.reported {
				t.Errorf("wrote %q for a session reported = %v; want %q, %v", out.String(), r.got.Session.Reported, tt.want, tt.reported)
			}
		})
	}
	t.Run("the use case receives the report as it arrived", func(t *testing.T) {
		t.Parallel()
		r := &renderer{}
		raw := `{"model":{"display_name":"Opus"}}`
		if err := (statusline.Handler{Render: r}).Handle(context.Background(), strings.NewReader(raw), io.Discard); err != nil {
			t.Fatal(err)
		}
		if string(r.got.Raw) != raw {
			t.Errorf("Raw = %q", r.got.Raw)
		}
	})
	t.Run("output that cannot be written is an error", func(t *testing.T) {
		t.Parallel()
		err := statusline.Handler{Render: &renderer{}}.Handle(context.Background(), strings.NewReader("{}"), brokenWriter{})
		if !errors.Is(err, errClosed) {
			t.Errorf("err = %v", err)
		}
	})
}
