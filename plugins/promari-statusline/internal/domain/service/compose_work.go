package service

import (
	"strconv"
	"strings"

	"promari-statusline/internal/domain/model"
)

const (
	// The longest texts shown before they are cut with an ellipsis.
	todoCharacters    = 20
	sessionCharacters = 40
	trackCharacters   = 40
)

// workChips shows the progress of the session: prompts, to-dos, tool calls.
func workChips(v *View) []model.Chip {
	var chips []model.Chip
	if activity, ok := v.Activity.Get(); ok && activity.Turns > 0 {
		chips = append(chips, chip(model.ToneMuted, "Turns ×"+strconv.Itoa(activity.Turns)))
	}
	if todos, ok := v.Facts.Todos.Get(); ok && todos.Total > 0 {
		c := chip(model.ToneGood, "✅ Todo "+strconv.Itoa(todos.Done)+"/"+strconv.Itoa(todos.Total))
		if todos.Doing != "" {
			c = append(c, space(), text(model.ToneMuted, "("+truncate(todos.Doing, todoCharacters)+")"))
		}
		chips = append(chips, c)
	}
	if tools, ok := v.Facts.Tools.Get(); ok && tools.Total > 0 {
		chips = append(chips, toolsChip(tools))
	}
	return chips
}

func toolsChip(tools model.ToolStats) model.Chip {
	top := make([]string, 0, len(tools.Top))
	for _, t := range tools.Top {
		top = append(top, t.Name+strconv.Itoa(t.Count))
	}
	c := chip(model.ToneMuted, "Tools ×"+strconv.Itoa(tools.Total)+" "+strings.Join(top, "/"))
	if tools.Errors > 0 {
		c = append(c, space(), text(model.ToneDanger, "❌ Err "+strconv.Itoa(tools.Errors)))
	}
	return c
}

// gitChips shows the branch, what is uncommitted or unpushed, and the pull
// request of the branch.
func gitChips(v *View) []model.Chip {
	git, ok := v.Facts.Git.Get()
	if !ok || git.Branch == "" {
		return nil
	}
	chips := []model.Chip{{{Text: git.Branch, Tone: model.ToneGood, Bold: true}}}
	for _, count := range []struct {
		n     int
		tone  model.Tone
		icon  string
		label string
	}{
		{git.Changed, model.ToneCaution, "📝", "Files"},
		{git.Ahead, model.ToneInfo, "🔼", "Ahead"},
		{git.Behind, model.ToneDanger, "🔽", "Behind"},
		{git.Stashes, model.ToneMuted, "📚", "Stash"},
	} {
		if count.n > 0 {
			chips = append(chips, chip(count.tone, count.icon+" "+strconv.Itoa(count.n)+" "+count.label))
		}
	}
	if !git.LastCommit.IsZero() {
		chips = append(chips, chip(model.ToneMuted, "📅 Cmt "+span(v.Now.Sub(git.LastCommit))))
	}
	if pull, ok := v.Facts.Pull.Get(); ok && pull.Number > 0 {
		chips = append(chips, pullChip(pull))
	}
	return chips
}

// pullChip shows a pull request: its number, the worst state of its checks,
// and the review decision.
func pullChip(pull model.PullRequest) model.Chip {
	c := model.Chip{
		text(model.TonePlain, "🔀 "),
		text(model.ToneMuted, "PR"),
		text(model.TonePlain, " #"+strconv.Itoa(pull.Number)),
	}
	switch {
	case pull.Failed > 0:
		c = append(c, space(), text(model.ToneDanger, "CI ❌ "+strconv.Itoa(pull.Failed)))
	case pull.Pending > 0:
		c = append(c, space(), text(model.ToneCaution, "CI ⏳ "+strconv.Itoa(pull.Pending)))
	case pull.Passed > 0:
		c = append(c, space(), text(model.ToneGood, "CI ✅ "+strconv.Itoa(pull.Passed)))
	}
	switch pull.Review {
	case model.ReviewApproved:
		c = append(c, space(), text(model.ToneGood, "approved"))
	case model.ReviewChangesRequested:
		c = append(c, space(), text(model.ToneDanger, "changes"))
	case model.ReviewRequired:
		c = append(c, space(), text(model.ToneCaution, "review"))
	}
	return c
}

// sessionChips shows the name of the session, which tells sessions apart when
// several run at once.
func sessionChips(v *View) []model.Chip {
	if v.Session.Name == "" {
		return nil
	}
	return []model.Chip{chip(model.ToneNote, truncate(v.Session.Name, sessionCharacters))}
}

// envChips shows what the session runs with: model, mode, repository, style.
func envChips(v *View) []model.Chip {
	s := v.Session
	name := s.Model
	if name == "" {
		name = "?"
	}
	chips := []model.Chip{{{Text: name, Tone: model.ToneAccent, Bold: true}}}
	var badges []string
	if s.Effort != "" {
		badges = append(badges, s.Effort)
	}
	if s.Thinking {
		badges = append(badges, "think")
	}
	if s.Fast {
		badges = append(badges, "fast")
	}
	if len(badges) > 0 {
		chips = append(chips, chip(model.ToneMuted, "Mode "+strings.Join(badges, "·")))
	}
	if s.Repo != "" {
		chips = append(chips, chip(model.ToneInfo, "📂 "+s.Repo))
	}
	if s.Style != "" {
		chips = append(chips, chip(model.ToneMuted, "Style "+s.Style))
	}
	return chips
}
