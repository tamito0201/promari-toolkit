package service

import (
	"strconv"
	"time"

	"promari-statusline/internal/domain/model"
)

const (
	// A ratio is shown only once its denominator is large enough to mean
	// something: a rate over the first minutes of a session is noise.
	focusMinObserved = time.Minute
	linesMinWorked   = 15 * time.Minute
	idleShown        = time.Minute

	// Tool error rates below errRateWarn are green, below errRateBad yellow.
	errRateWarn = 2.0
	errRateBad  = 5.0

	// cacheSaving is the share of the input price a cache read saves: a read
	// costs about a tenth of fresh input.
	cacheSaving = 90.0
)

// costChips shows what the session cost and the estimated spending around it.
func costChips(v *View) []model.Chip {
	var chips []model.Chip
	if cost, ok := v.Session.Cost.TotalUSD.Get(); ok {
		chips = append(chips, chip(model.ToneMoney, "Sess $"+fixed(cost, 2)))
	}
	spend, ok := v.Facts.Spend.Get()
	if !ok {
		return chips
	}
	if today, ok := spend.Today.Get(); ok {
		chips = append(chips, chip(model.ToneMoney, "Today $"+today.Text))
	}
	if block, ok := spend.Block.Get(); ok {
		c := chip(model.ToneMoney, "Blk $"+block.Text)
		if spend.BlockLeftText != "" {
			c = append(c, text(model.ToneMuted, " (残 "+spend.BlockLeftText+")"))
		}
		chips = append(chips, c)
	}
	if estimate, ok := spend.EstimatedBlock(); ok {
		chips = append(chips, chip(model.ToneMoney, "Est $"+grouped(estimate)))
	}
	return chips
}

// burnChips shows how fast money and time are spent.
func burnChips(v *View) []model.Chip {
	var chips []model.Chip
	if spend, ok := v.Facts.Spend.Get(); ok {
		if burn, ok := spend.BurnPerHour.Get(); ok {
			chips = append(chips, chip(model.ToneDanger, "$"+burn.Text+"/h"))
		}
	}
	if cost := v.Session.Cost; cost.Wall > 0 {
		s := "⏰ " + span(cost.Wall)
		if cost.API > 0 {
			s += " (API " + span(cost.API) + ")"
		}
		chips = append(chips, chip(model.ToneMuted, s))
	}
	if activity, ok := v.Activity.Get(); ok {
		chips = append(chips,
			chip(model.ToneGood, "Active "+span(activity.Worked())),
			chip(model.ToneMuted, "Streak "+span(activity.Streak(v.Now))))
		if activity.Idled() >= idleShown {
			chips = append(chips, chip(model.ToneMuted, "Idle "+span(activity.Idled())))
		}
	}
	return chips
}

// kpiChips shows what the session produced for its time and money.
func kpiChips(v *View) []model.Chip {
	var chips []model.Chip
	cost := v.Session.Cost
	added := float64(cost.LinesAdded)
	if cost.LinesAdded != 0 || cost.LinesRemoved != 0 {
		chips = append(chips, model.Chip{
			text(model.ToneMuted, "Lines "),
			text(model.ToneGood, "+"+strconv.Itoa(cost.LinesAdded)),
			text(model.ToneDanger, "-"+strconv.Itoa(cost.LinesRemoved)),
		})
	}
	activity, tracked := v.Activity.Get()
	if worked, idled := activity.Worked(), activity.Idled(); tracked && worked+idled > focusMinObserved {
		chips = append(chips, chip(model.ToneGood, "Focus "+fixed(worked.Seconds()/(worked+idled).Seconds()*percent, 0)+"%"))
	}
	if worked := activity.Worked(); tracked && added > 0 && worked > linesMinWorked {
		chips = append(chips, chip(model.ToneInfo, "Lines/h "+grouped(added/worked.Hours())))
	}
	total := cost.TotalUSD.Or(0)
	if total != 0 && added > 0 {
		chips = append(chips, chip(model.ToneMoney, "$/Line "+rate(total/added)))
	}
	if total != 0 && tracked && activity.Turns > 0 {
		chips = append(chips, chip(model.ToneMoney, "$/Turn "+rate(total/float64(activity.Turns))))
	}
	return chips
}

// perfChips shows how efficiently the session runs.
func perfChips(v *View) []model.Chip {
	var chips []model.Chip
	if cost := v.Session.Cost; cost.Wall > 0 && cost.API > 0 {
		// Above 1 the API worked longer than the clock ran: requests ran in parallel.
		chips = append(chips, chip(model.ToneNote, "Parallel ×"+fixed(cost.API.Seconds()/cost.Wall.Seconds(), 2)))
		if in := v.Session.Context.TotalInput; in > 0 {
			chips = append(chips, chip(model.ToneMuted, "Thruput "+grouped(in/cost.API.Seconds())+" tok/s"))
		}
	}
	if tools, ok := v.Facts.Tools.Get(); ok {
		if rate, ok := tools.ErrorRate(); ok {
			chips = append(chips, chip(severity(rate, errRateWarn, errRateBad), "ErrRate "+fixed(rate, 1)+"%"))
		}
	}
	return chips
}

// cacheChips shows the state of the prompt cache: how much of the input is read
// from it, what that saves, and what a cold start would cost.
func cacheChips(v *View) []model.Chip {
	cache := v.Session.Cache
	hit, ok := cache.HitRatio.Get()
	if !ok {
		return nil
	}
	c := chip(model.ToneNote, "Hit "+fixed(hit*percent, 0)+"%")
	if cache.Misses > 0 {
		c = append(c, space(), text(model.ToneCaution, strconv.Itoa(cache.Misses)+" Miss"))
		if cache.LastMissCause != "" {
			c = append(c, space(), text(model.ToneMuted, "("+cache.LastMissCause+")"))
		}
	}
	if cache.ExpiresAt.After(v.Now) {
		c = append(c, space(), text(model.ToneMuted, "残 "+until(cache.ExpiresAt, v.Now)))
	}
	chips := []model.Chip{c, chip(model.ToneNote, "Save "+fixed(hit*cacheSaving, 0)+"%")}
	if cache.RecacheTokens > 0 {
		chips = append(chips, chip(model.ToneMuted, "🧊 Cold "+tokens(cache.RecacheTokens)))
	}
	return chips
}

// tokenChips shows the tokens the session has moved.
func tokenChips(v *View) []model.Chip {
	var chips []model.Chip
	if window := v.Session.Context; window.TotalInput > 0 {
		chips = append(chips, chip(model.ToneMuted, "In "+tokens(window.TotalInput)+" / Out "+tokens(window.TotalOutput)))
	}
	if v.Session.Over200k {
		chips = append(chips, chip(model.ToneCaution, "🚧 200k超割増"))
	}
	if activity, ok := v.Activity.Get(); ok && activity.Compactions > 0 {
		chips = append(chips, chip(model.ToneCaution, "compact ×"+strconv.Itoa(activity.Compactions)))
	}
	return chips
}
