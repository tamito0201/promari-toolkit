package service

import (
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

func TestCells(t *testing.T) {
	t.Parallel()
	tests := []struct {
		text string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"残 116k", 7},       // a CJK character is two cells
		{"🧠 Context", 10},   // an emoji is two cells
		{"⚡ Claude", 9},     // drawn wide although Unicode lists it as narrow
		{"█░", 2},           // block elements are one cell each
		{" │ ", 3},          // the chip separator
		{"ｆｕｌｌ", 8},         // fullwidth forms
		{"✅ Todo 1/2", 11},  // wide by East Asian width, outside the emoji blocks
		{"é", 1},            // a precomposed letter
		{"$/Line 0.12", 11}, // plain ASCII
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			t.Parallel()
			if got := Cells(tt.text); got != tt.want {
				t.Errorf("Cells(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

func TestStartsWide(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]bool{"🧠 Context": true, "⚡ Claude": true, "Test": false, "": false, "残": true} {
		t.Run(text, func(t *testing.T) {
			t.Parallel()
			if got := StartsWide(text); got != want {
				t.Errorf("StartsWide(%q) = %v, want %v", text, got, want)
			}
		})
	}
}

func TestNumbers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"tokens below a thousand", tokens(950), "950"},
		{"tokens in thousands", tokens(84_000), "84k"},
		{"tokens at the thousand", tokens(1000), "1k"},
		{"tokens just under a million", tokens(999_499), "999k"},
		{"tokens in millions", tokens(1_250_000), "1.25M"},
		{"tokens of zero", tokens(0), "0"},
		{"grouped small", grouped(999), "999"},
		{"grouped thousands", grouped(1000), "1,000"},
		{"grouped millions", grouped(12_345_678), "12,345,678"},
		{"grouped negative", grouped(-1234.4), "-1,234"},
		{"grouped rounds", grouped(1999.6), "2,000"},
		{"fixed rounds half to even", fixed(0.125, 2), "0.12"},
		{"fixed", fixed(42, 0), "42"},
		{"plain drops a zero fraction", plain(300), "300"},
		{"plain keeps a fraction", plain(2.5), "2.5"},
		{"gib", gib(4.8*bytesPerGiB, 1), "4.8G"},
		{"gib without decimals", gib(80*bytesPerGiB, 0), "80G"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestSpan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0m"},
		{59 * time.Second, "0m"},
		{55 * time.Minute, "55m"},
		{time.Hour, "1h00m"},
		{4*time.Hour + 16*time.Minute, "4h16m"},
		{47*time.Hour + 59*time.Minute, "47h59m"},
		{48 * time.Hour, "2d0h"},
		{6*24*time.Hour + 12*time.Hour + 30*time.Minute, "6d12h"},
		{-time.Hour, "0m"}, // a clock that moved
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := span(tt.d); got != tt.want {
				t.Errorf("span(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestUntil(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"in the future", now.Add(4*time.Hour + 40*time.Minute), "4h40m"},
		{"now", now, "済"},
		{"in the past", now.Add(-time.Minute), "済"},
		{"less than a second away", now.Add(900 * time.Millisecond), "済"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := until(tt.at, now); got != tt.want {
				t.Errorf("until() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDates(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 1, 4, 9, 0, 0, time.UTC)
	if got := monthDay(at); got != "(10/1)" {
		t.Errorf("monthDay() = %q", got)
	}
	if got := clock(at); got != "04:09" {
		t.Errorf("clock() = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{"short enough", "abc", 3, "abc"},
		{"cut with an ellipsis", "abcdef", 4, "abc…"},
		{"counted in characters, not bytes", "日本語の文章", 4, "日本語…"},
		{"no space before the ellipsis", "ab cdef", 4, "ab…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := truncate(tt.text, tt.limit); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.text, tt.limit, got, tt.want)
			}
		})
	}
}

func TestSeverityAndBar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pct  float64
		tone model.Tone
		bar  string
	}{
		{0, model.ToneGood, "░░░░░░░░░░"},
		{42, model.ToneGood, "████░░░░░░"},
		{49.9, model.ToneGood, "█████░░░░░"},
		{50, model.ToneCaution, "█████░░░░░"},
		{79.9, model.ToneCaution, "████████░░"},
		{80, model.ToneDanger, "████████░░"},
		{100, model.ToneDanger, "██████████"},
		{250, model.ToneDanger, "██████████"}, // clamped
		{-5, model.ToneGood, "░░░░░░░░░░"},    // clamped
		{25, model.ToneGood, "██░░░░░░░░"},    // 2.5 cells round to the even 2
	}
	for _, tt := range tests {
		t.Run(tt.bar, func(t *testing.T) {
			t.Parallel()
			spans := bar(tt.pct, 10)
			if got := model.Chip(spans).Text(); got != tt.bar {
				t.Errorf("bar(%v) = %q, want %q", tt.pct, got, tt.bar)
			}
			if spans[0].Tone != tt.tone || spans[1].Tone != model.ToneMuted {
				t.Errorf("bar(%v) tones = %v, %v; want %v, muted", tt.pct, spans[0].Tone, spans[1].Tone, tt.tone)
			}
		})
	}
}

func TestNewer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"2.1.287", "2.1.34", true},
		{"2.1.34", "2.1.34", false},
		{"2.1.33", "2.1.34", false},
		{"3.0.0", "2.9.9", true},
		{"2.1.34.1", "2.1.34", true},
		{"2.1", "2.1.0", false},
		{"", "2.1.34", false},
		{"2.1.34", "", false},
		{"2.2.0-beta", "2.1.34", false}, // not compared
	}
	for _, tt := range tests {
		t.Run(tt.latest+" vs "+tt.current, func(t *testing.T) {
			t.Parallel()
			if got := Newer(tt.latest, tt.current); got != tt.want {
				t.Errorf("Newer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestRate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		v    float64
		want string
	}{
		{1234.5, "1,234"},
		{100, "100"},
		{99.94, "99.9"},
		{37.08, "37.1"},
		{10, "10.0"},
		{9.994, "9.99"},
		{2.4, "2.40"},
		{0.13, "0.13"},
		{0.1, "0.10"},
		// Below a tenth two significant digits are shown, so that a small rate is
		// not rounded to 0.00 ($37.08 for 10,522 lines is 0.0035 a line).
		{0.0996, "0.100"},
		{0.0104, "0.010"},
		{0.00352, "0.0035"},
		{0.000012, "0.000012"},
		{0.000001, "0.0000010"},
		{0.0000009, "0"},
		{0, "0"},
		{-1, "0"},
	}
	for _, tt := range tests {
		if got := rate(tt.v); got != tt.want {
			t.Errorf("rate(%v) = %q, want %q", tt.v, got, tt.want)
		}
	}
}
