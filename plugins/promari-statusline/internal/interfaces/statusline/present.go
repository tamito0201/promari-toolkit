package statusline

import (
	"strings"
	"time"

	"promari-statusline/internal/domain/model"
)

// SGR sequences. Only 256-colour foregrounds, one background and bold are
// used. Blink (SGR 5) and reverse (SGR 7) are not: the terminals that host
// Claude Code do not all draw them.
const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
	// band is the red band of an alarm: red background, white text.
	band = "\x1b[48;5;196m\x1b[38;5;231m"

	chipSeparator  = " │ "
	groupSeparator = " ┃ "
)

// colour returns the SGR sequence of a tone, or "" for plain text.
func colour(tone model.Tone) string {
	switch tone {
	case model.ToneAccent:
		return "\x1b[38;5;141m"
	case model.ToneGood:
		return "\x1b[38;5;114m"
	case model.ToneCaution:
		return "\x1b[38;5;214m"
	case model.ToneDanger:
		return "\x1b[38;5;203m"
	case model.ToneInfo:
		return "\x1b[38;5;75m"
	case model.ToneMoney:
		return "\x1b[38;5;222m"
	case model.ToneNote:
		return "\x1b[38;5;80m"
	case model.ToneMuted:
		return "\x1b[38;5;245m"
	case model.ToneBrand:
		return "\x1b[38;5;208m"
	case model.TonePlain:
		return ""
	}
	return ""
}

// Present writes the lines with ANSI colours.
//
// An alarm blinks without SGR 5: on odd seconds it is drawn as a red band, on
// even seconds in its own colours. Claude Code redraws the status line several
// times a second while a session is busy; when it is idle the alarm rests in
// one of the two frames, and it is visible in both.
func Present(lines []model.Line, at time.Time) string {
	flash := at.Unix()%2 != 0
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strings.Repeat(" ", line.Indent))
		for _, item := range line.Items {
			switch item.Sep {
			case model.SepChip:
				b.WriteString(colour(model.ToneMuted) + chipSeparator + reset)
			case model.SepGroup:
				b.WriteString(colour(model.ToneMuted) + groupSeparator + reset)
			case model.SepNone:
			}
			writeChip(&b, item.Chip, flash)
		}
	}
	return b.String()
}

// writeChip writes the spans of a chip. Neighbouring alarm spans are drawn as
// one band while the alarm flashes.
func writeChip(b *strings.Builder, chip model.Chip, flash bool) {
	for i := 0; i < len(chip); {
		span := chip[i]
		if !span.Alarm || !flash {
			writeSpan(b, span)
			i++
			continue
		}
		b.WriteString(band + bold)
		for ; i < len(chip) && chip[i].Alarm; i++ {
			b.WriteString(chip[i].Text)
		}
		b.WriteString(reset)
	}
}

func writeSpan(b *strings.Builder, span model.Span) {
	if span.Text == "" {
		return
	}
	style := colour(span.Tone)
	if span.Bold {
		style += bold
	}
	if style == "" {
		b.WriteString(span.Text)
		return
	}
	b.WriteString(style + span.Text + reset)
}
