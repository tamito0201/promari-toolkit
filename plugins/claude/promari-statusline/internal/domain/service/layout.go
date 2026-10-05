package service

import "promari-statusline/internal/domain/model"

const (
	// SeparatorCells is the width of " │ " between chips and " ┃ " between groups.
	SeparatorCells = 3
	// TightSeparatorCells is the width of "│" between the chips of a packed group.
	TightSeparatorCells = 1
	// labelColumn is the cell where every line's label starts. A line that
	// begins with an emoji (two cells and a space) reaches it by itself; a
	// line that begins with text is indented to it.
	labelColumn = 3
	// MinBudget is the narrowest line the layout plans for.
	MinBudget = 40
	// margin is kept free at the right edge of the terminal: Claude Code draws
	// the status line two cells indented and cuts a line that would touch the
	// last column, replacing its tail with an ellipsis. Measured 2026-10-05 at
	// 66 columns: a 64-cell packed line was shown as "…Est $2…" (62 cells and
	// an ellipsis) while a 63-cell line survived, so a line may use COLUMNS-3.
	margin = 3
)

// Budget returns the cells a line may use in a terminal of the given width.
func Budget(terminalCells int) int { return max(MinBudget, terminalCells-margin) }

// Layout packs the groups into lines no wider than budget cells.
//
// A group stays together. It joins the current line when it fits behind what
// is already there; otherwise it starts a new line. A group a little wider than
// a whole line is packed: its chips stand closer ("│" for " │ "), which keeps
// it on one line in a terminal a few cells too narrow. Only a group that does
// not fit even then is broken, between its chips. Its continuation lines carry
// no title: they hang under the chips of its first line, behind the same
// separator, so that each category is named once and its lines read as one
// block. A numbered title ("🚀 Perf 2") read as a category of its own.
func Layout(groups []model.Group, budget int) []model.Line {
	l := layouter{budget: budget}
	for _, g := range groups {
		if len(g.Chips) > 0 {
			l.place(g)
		}
	}
	l.flush()
	return l.lines
}

type layouter struct {
	budget int
	lines  []model.Line
	cur    model.Line
	// used is the width of cur, its indent included.
	used int
}

func (l *layouter) place(g model.Group) {
	unit := g.Chips
	if g.Title != "" {
		unit = append([]model.Chip{header(g)}, g.Chips...)
	}
	width := unitCells(unit)
	indent := indentOf(unit[0])
	switch {
	case len(l.cur.Items) > 0 && l.used+SeparatorCells+width <= l.budget:
		l.add(model.SepGroup, model.SepChip, unit, SeparatorCells+width)
	case indent+width <= l.budget:
		l.flush()
		l.cur.Indent = indent
		l.add(model.SepNone, model.SepChip, unit, indent+width)
	case indent+tightCells(unit) <= l.budget:
		l.flush()
		l.cur.Indent = indent
		l.add(model.SepNone, model.SepTight, unit, indent+tightCells(unit))
	default:
		l.flush()
		l.wrap(g)
	}
}

// add puts the chips of one unit on the current line: the first behind sep,
// the others behind between.
func (l *layouter) add(sep, between model.Separator, unit []model.Chip, cells int) {
	for i, c := range unit {
		if i > 0 {
			sep = between
		}
		l.cur.Items = append(l.cur.Items, model.Item{Sep: sep, Chip: c})
	}
	l.used += cells
}

// wrap breaks a group that is wider than a line between its chips. The first
// line starts with the title; each continuation line is indented to the end of
// the title and starts with the chip separator, so its chips stand under those
// of the first line. A chip too wide to hang there starts at the label column.
func (l *layouter) wrap(g model.Group) {
	hang := -1 // the indent of continuation lines; unknown before the first line
	for _, c := range g.Chips {
		width := Cells(c.Text())
		if len(l.cur.Items) > 0 && l.used+SeparatorCells+width > l.budget {
			l.flush()
		}
		switch {
		case len(l.cur.Items) > 0:
			l.add(model.SepChip, model.SepChip, []model.Chip{c}, SeparatorCells+width)
		case hang < 0:
			unit := []model.Chip{c}
			if g.Title != "" {
				unit = []model.Chip{header(g), c}
			}
			l.cur.Indent = indentOf(unit[0])
			l.add(model.SepNone, model.SepChip, unit, l.cur.Indent+unitCells(unit))
			hang = l.cur.Indent
			if g.Title != "" {
				hang += Cells(g.Title)
			}
		case g.Title != "" && hang+SeparatorCells+width <= l.budget:
			l.cur.Indent = hang
			l.add(model.SepChip, model.SepChip, []model.Chip{c}, hang+SeparatorCells+width)
		default:
			l.cur.Indent = indentOf(c)
			l.add(model.SepNone, model.SepChip, []model.Chip{c}, l.cur.Indent+width)
		}
	}
}

func (l *layouter) flush() {
	if len(l.cur.Items) > 0 {
		l.lines = append(l.lines, l.cur)
	}
	l.cur, l.used = model.Line{}, 0
}

// header returns the title chip of a group.
func header(g model.Group) model.Chip {
	return model.Chip{{Text: g.Title, Tone: g.Tone, Bold: true}}
}

// unitCells returns the width of chips joined by chip separators.
func unitCells(chips []model.Chip) int {
	cells := SeparatorCells * (len(chips) - 1)
	for _, c := range chips {
		cells += Cells(c.Text())
	}
	return cells
}

// tightCells returns the width of chips joined by tight separators.
func tightCells(chips []model.Chip) int {
	return unitCells(chips) - (SeparatorCells-TightSeparatorCells)*(len(chips)-1)
}

// indentOf returns the indent that brings a line starting with c to the label
// column.
func indentOf(c model.Chip) int {
	if StartsWide(c.Text()) {
		return 0
	}
	return labelColumn
}
