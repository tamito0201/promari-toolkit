package model

import "strings"

// Tone is the meaning of a piece of text. The presentation layer maps a tone
// to a colour; the domain only says what the text means.
type Tone uint8

// The tones of the status line.
const (
	TonePlain   Tone = iota // no meaning of its own
	ToneAccent              // the model, counts of parallel work
	ToneGood                // within limits
	ToneCaution             // approaching a limit
	ToneDanger              // over a limit, a failure
	ToneInfo                // neutral facts
	ToneMoney               // money
	ToneNote                // secondary highlights
	ToneMuted               // labels and context
	ToneBrand               // the Claude rate header
)

// Span is a run of text with one tone.
type Span struct {
	Text string
	Tone Tone
	Bold bool
	// Alarm marks a warning the reader must not miss. The presentation layer
	// makes it blink.
	Alarm bool
}

// Chip is one item of the status line: a few spans shown together and never
// broken across lines.
type Chip []Span

// Text returns the chip's text without any styling.
func (c Chip) Text() string {
	var b strings.Builder
	for _, s := range c {
		b.WriteString(s.Text)
	}
	return b.String()
}

// Alarmed returns the chip with every span marked as an alarm.
func (c Chip) Alarmed() Chip {
	out := make(Chip, len(c))
	for i, s := range c {
		s.Alarm = true
		out[i] = s
	}
	return out
}

// Group is a category of chips. A group stays together: on one line, or on
// consecutive lines that repeat its title.
type Group struct {
	// Title is the category's emoji and name. An empty title means the first
	// chip carries its own header.
	Title string
	Tone  Tone
	Chips []Chip
}

// Separator is what stands between two items of a line.
type Separator uint8

// The separators of a line.
const (
	SepNone  Separator = iota // the first item of a line
	SepChip                   // between two chips of a group
	SepGroup                  // between two groups
	SepTight                  // between two chips of a group that only fits a line when packed
)

// Item is a chip on a line, with the separator in front of it.
type Item struct {
	Sep  Separator
	Chip Chip
}

// Line is one line of the status line.
type Line struct {
	// Indent is the number of spaces in front of the line.
	Indent int
	Items  []Item
}
