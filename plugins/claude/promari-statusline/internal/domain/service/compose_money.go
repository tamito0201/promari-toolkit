package service

import (
	"maps"
	"slices"
	"strconv"
	"strings"
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
			chip(model.ToneMuted, "Streak "+span(activity.Streak(activityTime(&activity, v.Now)))))
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
		}, chip(model.ToneMuted, "Net "+signed(cost.LinesAdded-cost.LinesRemoved)))
	}
	activity, tracked := v.Activity.Get()
	if worked, idled := activity.Worked(), activity.Idled(); tracked && worked+idled > focusMinObserved {
		chips = append(chips, chip(model.ToneGood, "Focus "+fixed(worked.Seconds()/(worked+idled).Seconds()*percent, 0)+"%"))
	}
	if worked := activity.Worked(); tracked && added > 0 && worked > linesMinWorked {
		chips = append(chips, chip(model.ToneInfo, "Lines/h "+grouped(added/worked.Hours())))
	}
	if tracked {
		chips = append(chips, flowChips(&activity, activityTime(&activity, v.Now))...)
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

// flowChips shows the deep work of the session: the time in streaks long
// enough to count as deep work, the longest streak, and the breaks. The DevEx
// framework (Noda et al., 2023) names flow state as one of the three things
// that drive productivity; a streak shorter than model.DeepStreak ends before
// an interrupted task is back in full swing (Mark et al., CHI 2008).
func flowChips(a *model.Activity, now time.Time) []model.Chip {
	var chips []model.Chip
	if deep := a.Deep(now); deep > 0 {
		chips = append(chips, chip(model.ToneGood, "Deep "+span(deep)))
	}
	if longest := a.Longest(now); longest >= idleShown {
		chips = append(chips, chip(model.ToneMuted, "Longest "+span(longest)))
	}
	if a.Breaks > 0 {
		chips = append(chips, chip(model.ToneMuted, "Breaks ×"+strconv.Itoa(a.Breaks)))
	}
	return chips
}

// perfChips shows how efficiently the session runs.
func perfChips(v *View) []model.Chip {
	var chips []model.Chip
	cost := v.Session.Cost
	if cost.Wall > 0 && cost.API > 0 {
		// Above 1 the API worked longer than the clock ran: requests ran in parallel.
		chips = append(chips, chip(model.ToneNote, "Parallel ×"+fixed(cost.API.Seconds()/cost.Wall.Seconds(), 2)))
	}
	t, ok := v.Facts.Transcript.Get()
	if !ok {
		return chips
	}
	if c, ok := turnChip(t); ok {
		chips = append(chips, c)
	}
	if t.ThinkingSeconds >= idleShown.Seconds() {
		chips = append(chips, chip(model.ToneMuted, "Think time "+span(seconds(t.ThinkingSeconds))))
	}
	if rate, ok := t.Tools.ErrorRate(); ok {
		chips = append(chips, chip(severity(rate, errRateWarn, errRateBad), "ErrRate "+fixed(rate, 1)+"%"))
	}
	return chips
}

// cacheChips shows the state of the prompt cache: how much of the input is read
// from it, what that saves, and what a cold start would cost.
func cacheChips(v *View) []model.Chip {
	cache := v.Session.Cache
	if observed, ok := cache.Observed.Get(); ok && !observed {
		return []model.Chip{chip(model.ToneCaution, "Caching off")}
	}
	hit, ok := cache.HitRatio.Get()
	if !ok {
		return nil
	}
	c := chip(model.ToneNote, "Hit "+fixed(hit*percent, 0)+"%")
	if cache.TTL != "" {
		c = append(c, space(), text(model.ToneMuted, "TTL "+cache.TTL))
	}
	if warm, ok := cache.Warm.Get(); ok && !warm {
		c = append(c, space(), text(model.ToneCaution, "cold"))
	}
	if cache.Misses > 0 {
		c = append(c, space(), text(model.ToneCaution, strconv.Itoa(cache.Misses)+" Miss"))
		if cause := missNote(cache, v.Now); cause != "" {
			c = append(c, space(), text(model.ToneMuted, "("+cause+")"))
		}
	}
	if cache.ExpiresAt.After(v.Now) {
		c = append(c, space(), text(model.ToneMuted, "残 "+until(cache.ExpiresAt, v.Now)))
	}
	chips := []model.Chip{c, chip(model.ToneNote, "Save "+fixed(hit*cacheSaving, 0)+"%")}
	if cache.RecacheTokens > 0 {
		chips = append(chips, chip(model.ToneMuted, "🧊 Cold "+tokens(cache.RecacheTokens)))
	}
	if cache.WriteTokens > 0 {
		c := chip(model.ToneMuted, "Write "+tokens(cache.WriteTokens))
		if cache.MissTokens > 0 {
			c = append(c, text(model.ToneCaution, " (miss "+tokens(cache.MissTokens)+")"))
		}
		chips = append(chips, c)
	}
	if cache.Rebuilds > 0 {
		chips = append(chips, chip(model.ToneMuted, "Rebuild ×"+strconv.Itoa(cache.Rebuilds)))
	}
	if causes := missCauses(cache.MissCauses); causes != "" {
		chips = append(chips, chip(model.ToneCaution, "Causes "+causes))
	}
	return chips
}

// missNote describes the last miss: its cause and how long ago it was.
func missNote(cache model.PromptCache, now time.Time) string {
	note := cache.LastMissCause
	if !cache.LastMissAt.IsZero() {
		note = strings.TrimSpace(note + " " + span(now.Sub(cache.LastMissAt)) + " ago")
	}
	return note
}

// missCauses lists the causes of the misses, the most frequent first.
func missCauses(causes map[string]int) string {
	names := slices.Collect(maps.Keys(causes))
	slices.SortFunc(names, func(a, b string) int {
		if causes[a] != causes[b] {
			return causes[b] - causes[a]
		}
		return strings.Compare(a, b)
	})
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"×"+strconv.Itoa(causes[name]))
	}
	return strings.Join(parts, " ")
}

// tokenChips shows the tokens the session has moved.
func tokenChips(v *View) []model.Chip {
	var chips []model.Chip
	t, read := v.Facts.Transcript.Get()
	if sum := t.Tokens; read && sum.AllInput() > 0 {
		c := chip(model.ToneInfo, "Σ In "+tokens(sum.AllInput())+" / Out "+tokens(sum.Output))
		if cached, ok := sum.CachedShare(); ok {
			c = append(c, text(model.ToneMuted, " (cached "+fixed(cached, 0)+"%)"))
		}
		chips = append(chips, c)
		if share, ok := sum.ThinkingShare(); ok && sum.Thinking > 0 {
			chips = append(chips, chip(model.ToneMuted, "Think share "+fixed(share, 0)+"%"))
		}
		chips = append(chips, chip(model.ToneMuted, "Req ×"+strconv.Itoa(t.Requests)))
	}
	// The last request alone: how its input split between fresh tokens, tokens
	// written to the cache and tokens read from it, and what it wrote back.
	if window := v.Session.Context; window.Current > 0 {
		chips = append(chips, model.Chip{
			text(model.ToneMuted, "Last "),
			text(model.ToneInfo, "new "+tokens(window.Fresh)+" wr "+tokens(window.Written)+" rd "+tokens(window.Read)),
			text(model.ToneMuted, " → out "+tokens(window.TotalOutput)),
		})
	}
	if v.Session.Over200k {
		chips = append(chips, chip(model.ToneCaution, "🚧 200k超割増"))
	}
	return chips
}

// compactionChips shows how often the context was compacted and how long ago
// the last compaction was. The transcript records each compaction; without it
// they are guessed from the context shrinking between renders.
func compactionChips(v *View, t model.Transcript, read bool) []model.Chip {
	n := t.Compactions
	if !read || n == 0 {
		if activity, ok := v.Activity.Get(); ok {
			n = max(n, activity.Compactions)
		}
	}
	if n == 0 {
		return nil
	}
	c := chip(model.ToneCaution, "compact ×"+strconv.Itoa(n))
	if read && !t.LastCompaction.IsZero() {
		c = append(c, text(model.ToneMuted, " ("+span(v.Now.Sub(t.LastCompaction))+" ago)"))
	}
	return []model.Chip{c}
}
