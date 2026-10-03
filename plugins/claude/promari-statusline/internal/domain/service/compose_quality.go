package service

import (
	"strconv"
	"time"

	"promari-statusline/internal/domain/model"
)

// redTooLong is how long tests may stay red before it is an alarm: three
// quarters of the developers' test repairs were done within 25 minutes (Beller
// et al., "When, How, and Why Developers (Do Not) Test in Their IDEs",
// ESEC/FSE 2015).
const redTooLong = 25 * time.Minute

// qualityChips shows what the session's checks and the uncommitted change say
// about the quality of the work.
//
// No study gives a threshold for one developer's session, so the chips show
// the facts and colour only where a study measured where the trouble starts: a
// failed edit (Yang et al., "SWE-agent", NeurIPS 2024) and tests red for longer
// than most repairs take (Beller et al. 2015). A failing run is not a warning
// by itself: 65% of the developers' test runs failed in the same study.
func qualityChips(v *View) []model.Chip {
	var chips []model.Chip
	if t, ok := v.Facts.Transcript.Get(); ok {
		chips = append(chips, sessionQuality(v.Now, t.Quality)...)
	}
	if git, ok := v.Facts.Git.Get(); ok {
		chips = append(chips, changeQuality(git)...)
	}
	return chips
}

// sessionQuality shows the checks the session ran and how its edits went.
func sessionQuality(now time.Time, q model.Quality) []model.Chip {
	var chips []model.Chip
	if q.Tests.Runs > 0 {
		c := checkChip("Tests", q.Tests)
		if !q.RedSince.IsZero() {
			red := now.Sub(q.RedSince)
			c = append(c, text(model.ToneDanger, " red "+span(red)+" · "+strconv.Itoa(q.RedCalls)+" calls"))
			if red > model.SelfSolveLimit {
				// Past the time to work on it alone: ask someone.
				c = append(c, text(model.ToneCaution, " → ask"))
			}
			c = alarmIf(red > redTooLong, c)
		}
		chips = append(chips, c)
	}
	if q.Builds.Runs > 0 {
		chips = append(chips, checkChip("Build", q.Builds))
	}
	if q.Claims > 0 {
		chips = append(chips, chip(model.ToneCaution, "Claim≠ ×"+strconv.Itoa(q.Claims)))
	}
	if n := len(q.Unverified); n > 0 {
		chips = append(chips, chip(model.ToneCaution, "Untested "+strconv.Itoa(n)+" files"))
	}
	if q.EditFailures > 0 {
		c := chip(model.ToneMuted, "EditFail ×"+strconv.Itoa(q.EditFailures))
		if q.EditFailStreak > 0 {
			c = append(c, text(model.ToneCaution, " (streak "+strconv.Itoa(q.EditFailStreak)+")"))
		}
		chips = append(chips, c)
	}
	if q.Repeats > 0 {
		chips = append(chips, chip(model.ToneCaution, "Repeat ×"+strconv.Itoa(q.Repeats)))
	}
	return chips
}

// checkChip shows the runs of a check: how many, how the last ended, the share
// that failed and the failures a pipe hid from the exit status.
func checkChip(label string, runs model.CheckRuns) model.Chip {
	c := model.Chip{text(model.ToneNote, label+" ×"+strconv.Itoa(runs.Runs))}
	switch runs.Last {
	case model.OutcomePass:
		c = append(c, text(model.ToneGood, " ✅"))
	case model.OutcomeFail:
		c = append(c, text(model.ToneDanger, " ❌"))
	case model.OutcomeUnknown:
		c = append(c, text(model.ToneMuted, " ?"))
	}
	if rate, ok := runs.FailRate(); ok && runs.Failed > 0 {
		detail := " (fail " + fixed(rate, 0) + "%"
		if runs.Masked > 0 {
			detail += " · piped " + strconv.Itoa(runs.Masked)
		}
		c = append(c, text(model.ToneMuted, detail+")"))
	}
	return c
}

// changeQuality shows the risk of the uncommitted change: how widely it is
// spread, the share of it that is tests, the debt it admits, and the fixes of
// the day.
func changeQuality(git model.Git) []model.Chip {
	var chips []model.Chip
	if s, ok := model.ChangeSpread(git.Changes); ok {
		c := chip(model.ToneInfo, "Spread "+strconv.Itoa(s.Files)+"f · "+strconv.Itoa(s.Dirs)+"d · "+strconv.Itoa(s.Subsystems)+"s")
		if s.Files > 1 {
			c = append(c, text(model.ToneMuted, " H"+fixed(s.Entropy, 2)))
		}
		chips = append(chips, c)
	}
	if share, ok := model.TestShare(git.Changes); ok {
		tone := model.ToneMuted
		if share == 0 {
			tone = model.ToneCaution
		}
		chips = append(chips, chip(tone, "TestDiff "+fixed(share, 0)+"%"))
	}
	if git.DebtAdded+git.DebtRemoved > 0 {
		tone := model.ToneGood
		if git.DebtAdded > git.DebtRemoved {
			tone = model.ToneCaution
		}
		chips = append(chips, chip(tone, "Debt +"+strconv.Itoa(git.DebtAdded)+" -"+strconv.Itoa(git.DebtRemoved)))
	}
	if weakened := git.AssertsRemoved - git.AssertsAdded; git.SkipsAdded > 0 || weakened > 0 {
		c := chip(model.ToneDanger, "Weaken")
		if git.SkipsAdded > 0 {
			c = append(c, text(model.ToneDanger, " skip +"+strconv.Itoa(git.SkipsAdded)))
		}
		if weakened > 0 {
			c = append(c, text(model.ToneDanger, " assert -"+strconv.Itoa(weakened)))
		}
		chips = append(chips, c)
	}
	if git.MocksAdded > 0 {
		chips = append(chips, chip(model.ToneMuted, "Mocks +"+strconv.Itoa(git.MocksAdded)))
	}
	if git.DepsAdded > 0 {
		chips = append(chips, chip(model.ToneCaution, "Deps +"+strconv.Itoa(git.DepsAdded)))
	}
	if git.FixesToday > 0 {
		chips = append(chips, chip(model.ToneMuted, "Fix "+strconv.Itoa(git.FixesToday)+"/"+strconv.Itoa(git.CommitsToday)+" today"))
	}
	if git.AICommitsToday > 0 {
		chips = append(chips, chip(model.ToneMuted, "AI "+strconv.Itoa(git.AICommitsToday)+"/"+strconv.Itoa(git.CommitsToday)+" today"))
	}
	return chips
}
