package service

import (
	"strconv"

	"promari-statusline/internal/domain/model"
)

const (
	// SeparatorCells is the width of " │ " between chips and " ┃ " between groups.
	SeparatorCells = 3
	// labelColumn is the cell where every line's label starts. A line that
	// begins with an emoji (two cells and a space) reaches it by itself; a
	// line that begins with text is indented to it.
	labelColumn = 3
	// MinBudget is the narrowest line the layout plans for.
	MinBudget = 40
	// margin is kept free at the right edge of the terminal.
	margin = 2
)

// Budget returns the cells a line may use in a terminal of the given width.
func Budget(terminalCells int) int { return max(MinBudget, terminalCells-margin) }

// Layout packs the groups into lines no wider than budget cells.
//
// A group stays together. It joins the current line when it fits behind what
// is already there; otherwise it starts a new line. Only a group wider than a
// whole line is broken, between its chips, and each continuation repeats the
// group's title with a number ("🚀 Perf 2") so that every line still says
// what it shows.
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
		unit = append([]model.Chip{header(g, 1)}, g.Chips...)
	}
	width := unitCells(unit)
	indent := indentOf(unit[0])
	switch {
	case len(l.cur.Items) > 0 && l.used+SeparatorCells+width <= l.budget:
		l.add(model.SepGroup, unit, SeparatorCells+width)
	case indent+width <= l.budget:
		l.flush()
		l.cur.Indent = indent
		l.add(model.SepNone, unit, indent+width)
	default:
		l.flush()
		l.wrap(g)
	}
}

// add puts the chips of one unit on the current line, the first behind sep.
func (l *layouter) add(sep model.Separator, unit []model.Chip, cells int) {
	for i, c := range unit {
		if i > 0 {
			sep = model.SepChip
		}
		l.cur.Items = append(l.cur.Items, model.Item{Sep: sep, Chip: c})
	}
	l.used += cells
}

// wrap breaks a group that is wider than a line between its chips.
func (l *layouter) wrap(g model.Group) {
	part := 0
	for _, c := range g.Chips {
		width := Cells(c.Text())
		if len(l.cur.Items) > 0 && l.used+SeparatorCells+width > l.budget {
			l.flush()
		}
		if len(l.cur.Items) > 0 {
			l.add(model.SepChip, []model.Chip{c}, SeparatorCells+width)
			continue
		}
		part++
		unit := []model.Chip{c}
		if g.Title != "" {
			unit = []model.Chip{header(g, part), c}
		}
		l.cur.Indent = indentOf(unit[0])
		l.add(model.SepNone, unit, l.cur.Indent+unitCells(unit))
	}
}

func (l *layouter) flush() {
	if len(l.cur.Items) > 0 {
		l.lines = append(l.lines, l.cur)
	}
	l.cur, l.used = model.Line{}, 0
}

// header returns the title chip of a group; continuation lines are numbered.
func header(g model.Group, part int) model.Chip {
	title := g.Title
	if part > 1 {
		title += " " + strconv.Itoa(part)
	}
	return model.Chip{{Text: title, Tone: g.Tone, Bold: true}}
}

// unitCells returns the width of chips joined by chip separators.
func unitCells(chips []model.Chip) int {
	cells := SeparatorCells * (len(chips) - 1)
	for _, c := range chips {
		cells += Cells(c.Text())
	}
	return cells
}

// indentOf returns the indent that brings a line starting with c to the label
// column.
func indentOf(c model.Chip) int {
	if StartsWide(c.Text()) {
		return 0
	}
	return labelColumn
}
