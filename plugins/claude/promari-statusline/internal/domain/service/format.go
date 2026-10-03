package service

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"promari-statusline/internal/domain/model"
)

const (
	thousand = 1e3
	million  = 1e6
	percent  = 100.0

	minutesPerHour = 60
	hoursPerDay    = 24
	// longSpanHours is where a span switches from hours and minutes to days
	// and hours.
	longSpanHours = 48
	twoDigits     = 10
	groupSize     = 3

	bytesPerGiB = 1 << 30
	ellipsis    = "…"
)

// fixed formats a number with a fixed number of decimals, rounding half to even.
func fixed(v float64, decimals int) string {
	return strconv.FormatFloat(v, 'f', decimals, 64)
}

// Where rate changes the digits it shows.
const (
	rateWhole       = 100.0
	rateOneDecimal  = 10.0
	rateTwoDecimals = 0.1
	// rateSmallest is the smallest rate that is shown as a number; below it a
	// cost per unit is nothing.
	rateSmallest = 1e-6
)

// rate formats an amount per unit with the digits its size needs: 1,234, 37.1,
// 2.44, 0.13, and two significant digits below that (0.0035), so that a small
// rate is not rounded to 0.00.
func rate(v float64) string {
	switch {
	case v >= rateWhole:
		return grouped(v)
	case v >= rateOneDecimal:
		return fixed(v, 1)
	case v >= rateTwoDecimals:
		return fixed(v, 2)
	case v < rateSmallest:
		return "0"
	default:
		// 0.0035 has its first digit at the third decimal; one more is shown.
		return fixed(v, 1-int(math.Floor(math.Log10(v))))
	}
}

// plain formats a number with as few digits as it needs (3, not 3.0).
func plain(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// grouped formats a number without decimals and with thousands separators.
func grouped(v float64) string {
	digits := fixed(v, 0)
	sign := ""
	if rest, negative := strings.CutPrefix(digits, "-"); negative {
		sign, digits = "-", rest
	}
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%groupSize == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return sign + b.String()
}

// tokens formats a token count: 950, 84k, 1.25M.
func tokens(n float64) string {
	switch {
	case n >= million:
		return fixed(n/million, 2) + "M"
	case n >= thousand:
		return fixed(n/thousand, 0) + "k"
	default:
		return plain(n)
	}
}

// span formats a duration in its shortest form: 55m, 4h16m, 6d12h. A negative
// duration (a clock that moved) is shown as 0m.
func span(d time.Duration) string {
	minutes := int(max(0, d) / time.Minute)
	if minutes < minutesPerHour {
		return strconv.Itoa(minutes) + "m"
	}
	hours, minutes := minutes/minutesPerHour, minutes%minutesPerHour
	if hours >= longSpanHours {
		return strconv.Itoa(hours/hoursPerDay) + "d" + strconv.Itoa(hours%hoursPerDay) + "h"
	}
	pad := ""
	if minutes < twoDigits {
		pad = "0"
	}
	return strconv.Itoa(hours) + "h" + pad + strconv.Itoa(minutes) + "m"
}

// until formats the time left until t, or 済 once it has passed.
func until(t, now time.Time) string {
	left := t.Sub(now).Truncate(time.Second)
	if left <= 0 {
		return "済"
	}
	return span(left)
}

// monthDay formats a date as (M/D), to mark a value that is not from today.
func monthDay(t time.Time) string {
	return "(" + strconv.Itoa(int(t.Month())) + "/" + strconv.Itoa(t.Day()) + ")"
}

// clock formats a time of day as HH:MM.
func clock(t time.Time) string { return t.Format("15:04") }

// gib formats a number of bytes in GiB.
func gib(bytes float64, decimals int) string { return fixed(bytes/bytesPerGiB, decimals) + "G" }

// truncate shortens s to at most limit characters, ending with an ellipsis.
func truncate(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return strings.TrimRightFunc(string(runes[:limit-1]), unicode.IsSpace) + ellipsis
}

// text returns a span of one tone.
func text(tone model.Tone, s string) model.Span { return model.Span{Text: s, Tone: tone} }

// space is the gap between two parts of a chip.
func space() model.Span { return model.Span{Text: " "} }

// chip returns a chip of one tone.
func chip(tone model.Tone, s string) model.Chip { return model.Chip{text(tone, s)} }

const (
	// Usage turns yellow at warnPct and red at badPct.
	warnPct = 50.0
	badPct  = 80.0
)

// severity returns the tone of a percentage: good below warn, caution below
// bad, danger from bad on.
func severity(pct, warn, bad float64) model.Tone {
	switch {
	case pct < warn:
		return model.ToneGood
	case pct < bad:
		return model.ToneCaution
	default:
		return model.ToneDanger
	}
}

// bar draws a usage bar of the given width in cells.
func bar(pct float64, cells int) []model.Span {
	pct = max(0, min(percent, pct))
	filled := int(math.RoundToEven(pct / percent * float64(cells)))
	return []model.Span{
		text(severity(pct, warnPct, badPct), strings.Repeat("█", filled)),
		text(model.ToneMuted, strings.Repeat("░", cells-filled)),
	}
}
