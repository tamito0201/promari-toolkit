package service

import (
	"strconv"

	"promari-statusline/internal/domain/model"
)

// dueChips shows the work the user owes, the way the KPI books report it: the
// exceptions only (nothing late, nothing shows), red only where there is
// cause for concern, the current and the future before the past, and a count
// beside every rate. Late issues come first, as David Parmenter's most common
// weekly KPI is the list of late projects.
func dueChips(v *View) []model.Chip {
	w, ok := v.Facts.Workload.Get()
	if !ok {
		return nil
	}
	var chips []model.Chip
	if w.Overdue > 0 {
		c := chip(model.ToneDanger, "Overdue ×"+strconv.Itoa(w.Overdue))
		if w.LatestIssue > 0 {
			c = append(c, text(model.ToneMuted, " (#"+strconv.Itoa(w.LatestIssue)+" "+strconv.Itoa(w.MostLate)+"d)"))
		}
		chips = append(chips, c)
	}
	if w.Urgent > 0 {
		chips = append(chips, chip(model.ToneDanger, "Urgent >"+period(model.UrgentAfter)+" ×"+strconv.Itoa(w.Urgent)))
	}
	if !w.RedSince.IsZero() {
		red := v.Now.Sub(w.RedSince)
		c := chip(model.ToneDanger, "Base ✗ "+w.RedWorkflow+" "+span(red))
		if w.Red > 1 {
			c = append(c, text(model.ToneMuted, " (+"+strconv.Itoa(w.Red-1)+" more)"))
		}
		chips = append(chips, alarmIf(red > redTooLong, c))
	}
	if w.DueToday > 0 {
		chips = append(chips, chip(model.ToneCaution, "Due today ×"+strconv.Itoa(w.DueToday)))
	}
	if w.DueWeek > 0 {
		chips = append(chips, chip(model.ToneMuted, "Due 7d ×"+strconv.Itoa(w.DueWeek)))
	}
	if w.MyTurn > 0 {
		chips = append(chips, chip(model.ToneCaution, "My turn ×"+strconv.Itoa(w.MyTurn)))
	}
	if w.Stale > 0 {
		chips = append(chips, chip(model.ToneCaution, "Stale "+period(model.StaleAfter)+"+ ×"+strconv.Itoa(w.Stale)))
	}
	if w.Undated > 0 {
		chips = append(chips, chip(model.ToneMuted, "Undated "+strconv.Itoa(w.Undated)+"/"+atLeast(w.Issues, w.IssuesCapped)))
	}
	if w.Open > 0 {
		c := chip(model.ToneMuted, "WIP "+strconv.Itoa(w.Open)+" PR")
		if w.Drafts > 0 {
			c = append(c, text(model.ToneMuted, " ("+strconv.Itoa(w.Drafts)+" draft)"))
		}
		chips = append(chips, c)
	}
	chips = append(chips, deliveryChips(w)...)
	return chips
}

// atLeast writes a count, with "+" when the list it was counted from was cut
// at the most asked for and the count is only a lower bound.
func atLeast(n int, capped bool) string {
	if capped {
		return strconv.Itoa(n) + "+"
	}
	return strconv.Itoa(n)
}

// deliveryChips shows how the finished work flowed in the last two weeks: the
// median lead time from opening to merging, the share approved by the first
// review and the pull requests closed without merging. They are past measures
// and carry no colour; the first-pass share is coloured only from MinJudged
// reviewed pull requests, and its count is always shown.
func deliveryChips(w model.Workload) []model.Chip {
	var chips []model.Chip
	if w.Merged > 0 {
		chips = append(chips, chip(model.ToneMuted, "Lead p50 "+span(w.LeadP50)+" ×"+atLeast(w.Merged, w.MergedCapped)+"/"+period(model.FlowWindow)))
	}
	if w.Reviewed > 0 {
		tone := model.ToneMuted
		if w.Reviewed >= model.MinJudged && w.FirstPass*2 < w.Reviewed {
			tone = model.ToneCaution
		}
		chips = append(chips, chip(tone, "1st-pass "+strconv.Itoa(w.FirstPass)+"/"+strconv.Itoa(w.Reviewed)))
	}
	if w.Abandoned > 0 {
		chips = append(chips, chip(model.ToneMuted, "Abandoned ×"+strconv.Itoa(w.Abandoned)+"/"+period(model.FlowWindow)))
	}
	return chips
}
