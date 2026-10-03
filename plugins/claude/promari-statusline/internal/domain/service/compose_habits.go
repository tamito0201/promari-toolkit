package service

import (
	"strconv"
	"time"

	"promari-statusline/internal/domain/model"
)

// habitsChips shows how the repository is worked with, measured against the
// rules a training course for new engineers teaches its team exercise: a short
// branch named "kind/topic" cut from the default branch and never work on the
// default branch itself, a push within the day, commits of 20 to 50 lines with
// a message that says what changed, no build output or secrets added, no
// conflict left half resolved, merged branches deleted, a pull request with an
// assignee and a reviewer, reviewed within 30 minutes, and a steady run of
// days with commits.
func habitsChips(v *View) []model.Chip {
	var chips []model.Chip
	if git, ok := v.Facts.Git.Get(); ok && git.Branch != "" {
		chips = append(chips, branchHabits(v.Now, git)...)
		chips = append(chips, commitHabits(git)...)
	}
	if pull, ok := v.Facts.Pull.Get(); ok && pull.Number > 0 {
		chips = append(chips, reviewHabits(v.Now, pull)...)
	}
	if queue, ok := v.Facts.Reviews.Get(); ok && queue.Count > 0 {
		c := chip(model.ToneInfo, "To review ×"+strconv.Itoa(queue.Count))
		if !queue.Oldest.IsZero() {
			wait := v.Now.Sub(queue.Oldest)
			tone := model.ToneMuted
			if wait > model.ReviewTooSlow {
				tone = model.ToneCaution
			}
			c = append(c, text(tone, " (oldest "+span(wait)+")"))
		}
		chips = append(chips, c)
	}
	return chips
}

// branchHabits shows the habits of the branch: where the work is done, the
// branch's name and age, and what waits to be pushed.
func branchHabits(now time.Time, git model.Git) []model.Chip {
	var chips []model.Chip
	onDefault := model.IsDefaultBranch(git.Branch, git.DefaultBranch)
	if onDefault && git.Changed+git.Ahead > 0 {
		chips = append(chips, chip(model.ToneDanger, "On "+git.Branch+" directly").Alarmed())
	}
	// A branch's name matters to a team: it is judged only in a repository
	// with a remote whose default branch is known.
	if git.DefaultBranch != "" && !onDefault && !model.GoodBranchName(git.Branch) {
		chips = append(chips, chip(model.ToneCaution, "Name ✗ kind/topic"))
	}
	if !git.BranchStart.IsZero() {
		age := now.Sub(git.BranchStart)
		chips = append(chips, chip(ageTone(age, model.BranchTooOld, model.UnpushedTooOld), "Branch "+span(age)))
	}
	if !git.OldestUnpushed.IsZero() {
		wait := now.Sub(git.OldestUnpushed)
		chips = append(chips, alarmIf(wait > model.UnpushedTooOld, chip(ageTone(wait, model.UnpushedTooOld, model.UnpushedTooOld), "Unpushed "+span(wait))))
	}
	if git.MergedBranches > 0 {
		chips = append(chips, chip(model.ToneMuted, "Merged ×"+strconv.Itoa(git.MergedBranches)+" left"))
	}
	return chips
}

// ageTone colours an age: muted below soft, cautious below hard, then danger.
func ageTone(age, soft, hard time.Duration) model.Tone {
	switch {
	case age > hard:
		return model.ToneDanger
	case age > soft:
		return model.ToneCaution
	}
	return model.ToneMuted
}

// commitHabits shows the habits of the commits: their size and pace, their
// messages, and what should not have been added.
func commitHabits(git model.Git) []model.Chip {
	var chips []model.Chip
	if git.ConflictMarkers > 0 {
		chips = append(chips, chip(model.ToneDanger, "Conflict marks ×"+strconv.Itoa(git.ConflictMarkers)).Alarmed())
	}
	if git.Junk > 0 {
		tone := model.ToneCaution
		if git.JunkStaged > 0 {
			tone = model.ToneDanger
		}
		chips = append(chips, chip(tone, "Junk ×"+strconv.Itoa(git.Junk)))
	}
	if size, ok := git.CommitSize(); ok {
		c := model.Chip{text(commitSizeTone(size), fixed(size, 0)+" L/cmt")}
		c = append(c, text(paceTone(git.CommitsToday), " ×"+strconv.Itoa(git.CommitsToday)+"/day"))
		if git.LargestToday > model.LargeCommit {
			c = append(c, text(model.ToneCaution, " max "+strconv.Itoa(git.LargestToday)))
		}
		if ratio, ok := git.AddDeleteRatio(); ok {
			c = append(c, text(model.ToneMuted, " +/- "+fixed(ratio, 1)))
		}
		chips = append(chips, c)
	}
	if git.BehindDefault > 0 {
		chips = append(chips, chip(model.ToneCaution, "Behind "+git.DefaultBranch+" ×"+strconv.Itoa(git.BehindDefault)))
	}
	// Only a team that follows Conventional Commits is held to them: one of
	// today's commits follows them.
	if git.ConventionalToday > 0 {
		tone := model.ToneCaution
		if git.ConventionalToday == git.CommitsToday {
			tone = model.ToneGood
		}
		chips = append(chips, chip(tone, "Conv "+strconv.Itoa(git.ConventionalToday)+"/"+strconv.Itoa(git.CommitsToday)))
	}
	if git.VagueToday > 0 {
		chips = append(chips, chip(model.ToneCaution, "Vague msg ×"+strconv.Itoa(git.VagueToday)))
	}
	if git.Streak > 0 {
		tone := model.ToneMuted
		if git.Streak >= model.SteadyStreak {
			tone = model.ToneGood
		}
		chips = append(chips, model.Chip{
			text(tone, "Streak "+strconv.Itoa(git.Streak)+"d"),
			text(model.ToneMuted, " (max "+strconv.Itoa(git.LongestStreak)+"d)"),
		})
	}
	return chips
}

// commitSizeTone colours the lines per commit: 20 to 50 is a standard step,
// fewer than 10 a small fix, more than 100 a large change.
func commitSizeTone(size float64) model.Tone {
	switch {
	case size > model.LargeCommit:
		return model.ToneCaution
	case size < model.SmallCommit:
		return model.ToneMuted
	}
	return model.ToneGood
}

// paceTone colours the commits of the day: two to five is a standard pace.
func paceTone(commits int) model.Tone {
	if commits >= model.CommitsPerDayLow && commits <= model.CommitsPerDayHigh {
		return model.ToneGood
	}
	return model.ToneMuted
}

// reviewHabits shows the habits of the pull request: someone assigned, a
// reviewer asked, and a review that does not keep the author waiting.
func reviewHabits(now time.Time, pull model.PullRequest) []model.Chip {
	var chips []model.Chip
	if !pull.PeopleKnown {
		return nil
	}
	if pull.Assignees == 0 {
		chips = append(chips, chip(model.ToneCaution, "No assignee"))
	}
	if pull.Reviewers+pull.Reviews == 0 && !pull.Draft {
		chips = append(chips, chip(model.ToneCaution, "No reviewer"))
	}
	if pull.Reviews == 0 && !pull.Draft && !pull.Created.IsZero() && pull.Review != model.ReviewApproved {
		if wait := now.Sub(pull.Created); wait > model.ReviewTooSlow {
			chips = append(chips, chip(model.ToneCaution, "Review wait "+span(wait)))
		}
	}
	return chips
}
