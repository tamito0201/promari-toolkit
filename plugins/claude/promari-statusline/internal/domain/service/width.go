// Package service holds the status line's rules: how wide text is on a
// terminal, how values are written, which chips a set of facts becomes, and
// how chips are laid out in lines. Every function is pure: the same input
// gives the same lines, and nothing here reads a clock, a file or a process.
package service

import (
	"strings"

	"golang.org/x/text/width"
)

const (
	// wideCells is the width of an emoji or a CJK character.
	wideCells = 2
	// emojiFirst and emojiLast bound the supplementary emoji blocks, which
	// terminals draw two cells wide whatever their East Asian width says.
	emojiFirst = 0x1F000
	emojiLast  = 0x1FAFF
	// wideSymbols are drawn two cells wide although Unicode lists them as narrow.
	wideSymbols = "⚡♨⌛⚑⇅♻⏱⚠"
)

// Cells returns the display width of s in terminal cells. A terminal cuts a
// status line at its width without any error, so every width in the layout is
// counted here, never with len: an emoji or a CJK character takes two cells.
func Cells(s string) int {
	cells := 0
	for _, r := range s {
		cells += runeCells(r)
	}
	return cells
}

// StartsWide reports whether the first character of s takes two cells.
func StartsWide(s string) bool {
	for _, r := range s {
		return runeCells(r) == wideCells
	}
	return false
}

func runeCells(r rune) int {
	if r >= emojiFirst && r <= emojiLast || strings.ContainsRune(wideSymbols, r) {
		return wideCells
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return wideCells
	default:
		return 1
	}
}
