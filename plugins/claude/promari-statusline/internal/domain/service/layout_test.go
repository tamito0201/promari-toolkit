package service

import (
	"slices"
	"strings"
	"testing"

	"promari-statusline/internal/domain/model"
)

// draw writes lines the way a terminal shows them, without colours.
func draw(lines []model.Line) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		var b strings.Builder
		b.WriteString(strings.Repeat(" ", line.Indent))
		for _, item := range line.Items {
			switch item.Sep {
			case model.SepChip:
				b.WriteString(" │ ")
			case model.SepGroup:
				b.WriteString(" ┃ ")
			case model.SepTight:
				b.WriteString("│")
			case model.SepNone:
			}
			b.WriteString(item.Chip.Text())
		}
		out = append(out, b.String())
	}
	return out
}

func group(title string, chips ...string) model.Group {
	g := model.Group{Title: title}
	for _, c := range chips {
		g.Chips = append(g.Chips, model.Chip{{Text: c}})
	}
	return g
}

func TestLayout(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		groups []model.Group
		budget int
		want   []string
	}{
		{"no groups", nil, 40, nil},
		{"a group without chips is left out", []model.Group{group("🧠 A")}, 40, nil},
		{
			"groups that fit share a line",
			[]model.Group{group("🧠 A", "one"), group("💰 B", "two")},
			40,
			[]string{"🧠 A │ one ┃ 💰 B │ two"},
		},
		{
			"a group that does not fit behind the line starts a new one",
			[]model.Group{group("🧠 A", "0123456789"), group("💰 B", "0123456789")},
			30,
			[]string{"🧠 A │ 0123456789", "💰 B │ 0123456789"},
		},
		{
			"a line may be exactly as wide as the budget",
			// "🧠 A │ 01234" is 12 cells, the separator 3, "💰 B │ 01234" 12: 27.
			[]model.Group{group("🧠 A", "01234"), group("💰 B", "01234")},
			27,
			[]string{"🧠 A │ 01234 ┃ 💰 B │ 01234"},
		},
		{
			"one cell too many breaks the line",
			[]model.Group{group("🧠 A", "01234"), group("💰 B", "01234")},
			26,
			[]string{"🧠 A │ 01234", "💰 B │ 01234"},
		},
		{
			"a group wider than a line wraps between its chips and hangs its continuation under them",
			[]model.Group{group("🚀 Perf", "aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc")},
			24,
			[]string{"🚀 Perf │ aaaaaaaaaa", "        │ bbbbbbbbbb", "        │ cccccccccc"},
		},
		{
			"a wrapped group fills each line as far as it goes",
			[]model.Group{group("🚀 Perf", "aaaa", "bbbb", "cccc", "dddd")},
			24,
			[]string{"🚀 Perf │ aaaa │ bbbb", "        │ cccc │ dddd"},
		},
		{
			"the next group joins the last line of a wrapped group",
			// "        │ bbbbbbbbbbbbbbb" is 25 cells, the separator 3, "🌿 G │ x" 8: 36.
			// (Packed, the first group would be 39.)
			[]model.Group{group("🚀 Perf", "aaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbb"), group("🌿 G", "x")},
			38,
			[]string{"🚀 Perf │ aaaaaaaaaaaaaaa", "        │ bbbbbbbbbbbbbbb ┃ 🌿 G │ x"},
		},
		{
			"a chip too wide to hang under the title starts at the label column",
			// Hung, "        │ " and the chip would be 27 cells.
			[]model.Group{group("🚀 Perf", "aaaa", "bbbbbbbbbbbbbbbbb")},
			24,
			[]string{"🚀 Perf │ aaaa", "   bbbbbbbbbbbbbbbbb"},
		},
		{
			"a group without a title wraps at its own indent",
			[]model.Group{{Chips: []model.Chip{{{Text: "⚡ Claude aaaaaaaa"}}, {{Text: "bbbbbbbbbbbb"}}}}},
			24,
			[]string{"⚡ Claude aaaaaaaa", "   bbbbbbbbbbbb"},
		},
		// "🚀 Perf │ aaaaaaaaaaaa │ bbbbbbbbbbbb" is 37 cells; packed it is 33.
		{
			"a group that fits a line keeps its separators",
			[]model.Group{group("🚀 Perf", "aaaaaaaaaaaa", "bbbbbbbbbbbb")},
			37,
			[]string{"🚀 Perf │ aaaaaaaaaaaa │ bbbbbbbbbbbb"},
		},
		{
			"a group one cell too wide is packed onto the line",
			[]model.Group{group("🚀 Perf", "aaaaaaaaaaaa", "bbbbbbbbbbbb")},
			36,
			[]string{"🚀 Perf│aaaaaaaaaaaa│bbbbbbbbbbbb"},
		},
		{
			"a packed group may fill the line to its last cell",
			[]model.Group{group("🚀 Perf", "aaaaaaaaaaaa", "bbbbbbbbbbbb")},
			33,
			[]string{"🚀 Perf│aaaaaaaaaaaa│bbbbbbbbbbbb"},
		},
		{
			"a group too wide even when packed is broken, with its usual separators",
			[]model.Group{group("🚀 Perf", "aaaaaaaaaaaa", "bbbbbbbbbbbb")},
			32,
			[]string{"🚀 Perf │ aaaaaaaaaaaa", "        │ bbbbbbbbbbbb"},
		},
		{
			"a packed group starts a line of its own, and the next group does not join a line that is full",
			[]model.Group{group("🧠 A", "one"), group("🚀 Perf", "aaaaaaaaaaaa", "bbbbbbbbbbbb"), group("🌿 G", "x")},
			36,
			[]string{"🧠 A │ one", "🚀 Perf│aaaaaaaaaaaa│bbbbbbbbbbbb", "🌿 G │ x"},
		},
		{
			"a packed group without a title is indented like any other",
			[]model.Group{group("", "aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb")},
			37,
			[]string{"   aaaaaaaaaaaaaaaa│bbbbbbbbbbbbbbbb"},
		},
		{
			"a group without a title that starts with text is indented to the label column",
			[]model.Group{group("", "plain text")},
			40,
			[]string{"   plain text"},
		},
		{
			"a group without a title that starts with an emoji is not indented",
			[]model.Group{group("", "⚡ Claude 5h 1%")},
			40,
			[]string{"⚡ Claude 5h 1%"},
		},
		{
			"the indent counts against the budget",
			// "plain text" is 10 cells; with the indent of 3 it needs 13.
			[]model.Group{group("🧠 A", "x"), group("", "plain text", "more")},
			12,
			[]string{"🧠 A │ x", "   plain text", "   more"},
		},
		{
			"a headerless group that wraps keeps no header",
			[]model.Group{group("", "⚡ aaaaaaaaaaaaaaaaaa", "⚡ bbbbbbbbbbbbbbbbbb")},
			24,
			[]string{"⚡ aaaaaaaaaaaaaaaaaa", "⚡ bbbbbbbbbbbbbbbbbb"},
		},
		{
			"a chip wider than the budget still gets its line",
			[]model.Group{group("🧠 A", "0123456789012345678901234567890123456789")},
			20,
			[]string{"🧠 A │ 0123456789012345678901234567890123456789"},
		},
		{
			"width is counted in cells, not characters",
			// "残り残り残り" is 6 characters and 12 cells.
			[]model.Group{group("🧠 A", "残り残り残り"), group("💰 B", "x")},
			27,
			[]string{"🧠 A │ 残り残り残り", "💰 B │ x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lines := Layout(tt.groups, tt.budget)
			got := draw(lines)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Layout() =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(tt.want, "\n  "))
			}
		})
	}
}

// No line may be wider than the budget unless a single chip (with its header)
// is: the terminal would cut it without a word.
func TestLayoutNeverOverflows(t *testing.T) {
	t.Parallel()
	groups := []model.Group{
		group("🧠 Context", "████░░░░░░ 42% 84k/200k 残 116k"),
		group("", "⚡ Claude 5h ░░░░░ 1% 🔄 4h40m 7d ████░ 73% 🔄 4d7h Pace ×1.9"),
		group("💰 Cost", "Sess $1.23", "Today $45.67", "Blk $8.90 (残 2h15m)", "Est $16"),
		group("🔥 Burn", "$3.21/h", "⏰ 10m (API 5m)", "Active 0m", "Streak 0m"),
		group("📈 KPI", "Lines +10-2", "Focus 75%", "Lines/h 1,200", "$/Line 0.12", "$/Turn 0.5"),
		group("🌿 Git", "develop", "📝 2 Files", "🔽 16 Behind", "📅 Cmt 21h28m"),
		group("💻 System", "🕐 04:09", "CPU 3.9/10c", "🧮 Mem 4.8G", "💾 Disk 80G", "🔌 Bat 100%"),
	}
	for budget := MinBudget; budget <= 200; budget++ {
		for _, line := range Layout(groups, budget) {
			width := line.Indent
			widest := 0
			for _, item := range line.Items {
				switch item.Sep {
				case model.SepChip, model.SepGroup:
					width += SeparatorCells
				case model.SepTight:
					width += TightSeparatorCells
				case model.SepNone:
				}
				cells := Cells(item.Chip.Text())
				width += cells
				widest = max(widest, cells)
			}
			// A line of one header and one chip cannot be made narrower.
			if width > budget && len(line.Items) > 2 {
				t.Fatalf("budget %d: a line of %d items is %d cells wide: %q", budget, len(line.Items), width, draw([]model.Line{line})[0])
			}
		}
	}
}

func TestBudget(t *testing.T) {
	t.Parallel()
	// Claude Code draws the status line two cells indented and cuts a line that
	// would touch the last column, replacing its tail with an ellipsis. Measured
	// 2026-10-05: at 66 columns a 64-cell line was shown as 62 cells plus "…",
	// while a 63-cell line survived, so only COLUMNS-3 cells are safe.
	for cells, want := range map[int]int{100: 97, 87: 84, 66: 63, 44: 41, 43: 40, 0: 40} {
		if got := Budget(cells); got != want {
			t.Errorf("Budget(%d) = %d, want %d", cells, got, want)
		}
	}
}
