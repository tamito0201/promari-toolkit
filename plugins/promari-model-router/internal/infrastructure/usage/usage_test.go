package usage_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/clock"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/usage"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/pkg/fp"
)

var (
	now     = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	optCmp  = cmp.AllowUnexported(fp.Option[float64]{})
	fresh   = now.Add(-10 * time.Minute)
	stale   = now.Add(-2 * time.Hour)
	future  = now.Add(time.Hour).Unix()
	expired = now.Add(-time.Minute).Unix()
)

func settings(dir string) model.PressureSettings {
	return model.PressureSettings{
		ClaudeRateFile: filepath.Join(dir, "claude.json"), CodexRateFile: filepath.Join(dir, "codex.json"),
		FiveHourHigh: 80, SevenDayHigh: 90, CodexBlockPercent: 95, MaxAgeHours: 1,
	}
}

func write(t *testing.T, path, body string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func claudeCache(ts time.Time, rl string) string {
	return `{"ts":` + unix(ts.Unix()) + `,"rl":` + rl + `}`
}

func unix(n int64) string { return strconv.FormatInt(n, 10) }

func TestClaude(t *testing.T) {
	tests := []struct {
		name string
		body *string // nil: no cache file
		want model.Pressure
	}{
		{name: "no cache file is unknown", want: model.Pressure{}},
		{name: "broken cache is unknown", body: new("{"), want: model.Pressure{}},
		{name: "stale cache is unknown", body: new(claudeCache(stale, `{"five_hour":{"used_percentage":99}}`)), want: model.Pressure{}},
		{
			name: "fresh and below both thresholds",
			body: new(claudeCache(fresh, `{"five_hour":{"used_percentage":50,"resets_at":`+unix(future)+`},"seven_day":{"used_percentage":20}}`)),
			want: model.Pressure{Known: true, FiveHour: fp.Some(50.0), SevenDay: fp.Some(20.0)},
		},
		{
			name: "five-hour window at the threshold is high",
			body: new(claudeCache(fresh, `{"five_hour":{"used_percentage":80},"seven_day":{"used_percentage":20}}`)),
			want: model.Pressure{Known: true, High: true, FiveHour: fp.Some(80.0), SevenDay: fp.Some(20.0)},
		},
		{
			name: "seven-day window above the threshold is high",
			body: new(claudeCache(fresh, `{"seven_day":{"used_percent":95}}`)),
			want: model.Pressure{Known: true, High: true, SevenDay: fp.Some(95.0)},
		},
		{
			name: "used_percent is read when used_percentage is absent",
			body: new(claudeCache(fresh, `{"five_hour":{"used_percent":79.5}}`)),
			want: model.Pressure{Known: true, FiveHour: fp.Some(79.5)},
		},
		{
			name: "a cache exactly max_age old is still fresh",
			body: new(claudeCache(now.Add(-time.Hour), `{"five_hour":{"used_percentage":50}}`)),
			want: model.Pressure{Known: true, FiveHour: fp.Some(50.0)},
		},
		{
			name: "used_percentage is preferred over used_percent",
			body: new(claudeCache(fresh, `{"five_hour":{"used_percentage":90,"used_percent":1}}`)),
			want: model.Pressure{Known: true, High: true, FiveHour: fp.Some(90.0)},
		},
		{
			name: "a window past its reset is ignored",
			body: new(claudeCache(fresh, `{"five_hour":{"used_percentage":99,"resets_at":`+unix(expired)+`}}`)),
			want: model.Pressure{Known: true},
		},
		{
			name: "a window without a percentage is absent",
			body: new(claudeCache(fresh, `{"five_hour":{"resets_at":`+unix(future)+`}}`)),
			want: model.Pressure{Known: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := settings(t.TempDir())
			if tt.body != nil {
				write(t, cfg.ClaudeRateFile, *tt.body, fresh)
			}
			got := usage.New(cfg, clock.Fixed{At: now}).Claude()
			if diff := cmp.Diff(tt.want, got, optCmp); diff != "" {
				t.Errorf("Claude() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCodex(t *testing.T) {
	tests := []struct {
		name  string
		body  *string // nil: no cache file
		mtime time.Time
		want  model.CodexQuota
	}{
		{name: "no cache file counts as available", want: model.CodexQuota{Available: true}},
		{name: "stale cache counts as available", body: new(`{"rl":{"primary":{"used_percent":99}}}`), mtime: stale, want: model.CodexQuota{Available: true}},
		{
			name: "a cache exactly max_age old is still read", body: new(`{"rl":{"primary":{"used_percent":99}}}`), mtime: now.Add(-time.Hour),
			want: model.CodexQuota{Used: fp.Some(99.0)},
		},
		{name: "broken cache counts as available", body: new("not json"), mtime: fresh, want: model.CodexQuota{Available: true}},
		{
			name: "below the block percent is available", body: new(`{"rl":{"primary":{"used_percent":50}}}`), mtime: fresh,
			want: model.CodexQuota{Available: true, Used: fp.Some(50.0)},
		},
		{
			name: "at the block percent is blocked", body: new(`{"rl":{"primary":{"used_percentage":95}}}`), mtime: fresh,
			want: model.CodexQuota{Used: fp.Some(95.0)},
		},
		{
			name: "no primary window is available", body: new(`{"rl":{}}`), mtime: fresh,
			want: model.CodexQuota{Available: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := settings(t.TempDir())
			if tt.body != nil {
				write(t, cfg.CodexRateFile, *tt.body, tt.mtime)
			}
			got := usage.New(cfg, clock.Fixed{At: now}).Codex()
			if diff := cmp.Diff(tt.want, got, optCmp); diff != "" {
				t.Errorf("Codex() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
