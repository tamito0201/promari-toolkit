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
	t, ok := v.Facts.Transcript.Get()
	if !ok {
		return chips
	}
	if t.Tools.Total > 0 {
		chips = append(chips, toolsChip(t.Tools))
	}
	if n := len(t.Files); n > 0 {
		chips = append(chips, chip(model.ToneInfo, "Edited "+strconv.Itoa(n)+" files"))
	}
	if t.Hooks > 0 {
		c := chip(model.ToneMuted, "Hooks ×"+strconv.Itoa(t.Hooks))
		if t.HookErrors > 0 {
			c = append(c, space(), text(model.ToneDanger, "❌ "+strconv.Itoa(t.HookErrors)))
		}
		chips = append(chips, c)
	}
	if t.Diagnostics > 0 {
		chips = append(chips, chip(model.ToneCaution, "Diag ×"+strconv.Itoa(t.Diagnostics)))
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
	if wt := worktreeName(&v.Session); wt != "" {
		chips = append(chips, chip(model.ToneNote, "WT "+wt))
	}
	if git.Operation != "" {
		chips = append(chips, chip(model.ToneDanger, strings.ToUpper(git.Operation)+" in progress"))
	}
	if git.Conflicts > 0 {
		chips = append(chips, chip(model.ToneDanger, "Conflict ×"+strconv.Itoa(git.Conflicts)).Alarmed())
	}
	for _, count := range []struct {
		n     int
		tone  model.Tone
		icon  string
		label string
	}{
		{git.Changed, model.ToneCaution, "📝", "Files"},
		{git.Staged, model.ToneGood, "➕", "Staged"},
		{git.Untracked, model.ToneMuted, "❓", "New"},
		{git.Ahead, model.ToneInfo, "🔼", "Ahead"},
		{git.Behind, model.ToneDanger, "🔽", "Behind"},
		{git.Stashes, model.ToneMuted, "📚", "Stash"},
	} {
		if count.n > 0 {
			chips = append(chips, chip(count.tone, count.icon+" "+strconv.Itoa(count.n)+" "+count.label))
		}
	}
	if size := git.Inserted + git.Deleted; size > 0 {
		chips = append(chips, model.Chip{
			text(model.ToneMuted, "Diff "),
			text(reviewTone(size), "+"+grouped(float64(git.Inserted))+" -"+grouped(float64(git.Deleted))),
		})
	}
	if !git.LastCommit.IsZero() {
		c := chip(model.ToneMuted, "📅 Cmt "+span(v.Now.Sub(git.LastCommit)))
		if git.CommitsToday > 0 {
			c = append(c, text(model.ToneInfo, " (×"+strconv.Itoa(git.CommitsToday)+" today)"))
		}
		chips = append(chips, c)
	}
	return chips
}

// The sizes of a change, in lines, from which a review gets worse. A review of
// more than 400 lines finds fewer defects (the SmartBear study of Cisco's code
// reviews, Cohen 2006: review 200-400 lines at a time); changes of more than
// 1,000 lines waited a day for review at Google (Sadowski et al., "Modern Code
// Review: A Case Study at Google", ICSE-SEIP 2018).
const (
	reviewWarnLines = 400
	reviewBadLines  = 1000
)

// reviewTone colours the size of a change by how well it can be reviewed.
func reviewTone(lines int) model.Tone {
	switch {
	case lines > reviewBadLines:
		return model.ToneDanger
	case lines > reviewWarnLines:
		return model.ToneCaution
	default:
		return model.ToneGood
	}
}

// worktreeName returns the worktree the session runs in: the worktree session's
// name, or the linked git worktree; "" for the main working tree.
func worktreeName(s *model.Session) string {
	if wt, ok := s.Worktree.Get(); ok {
		return wt.Name
	}
	return s.GitWorktree
}

// pullChips shows the pull request of the branch: its number, the worst state
// of its checks, and the review decision. It is a category of its own, not a
// chip of Git: behind a long branch name it would push Git onto a second line.
func pullChips(v *View) []model.Chip {
	// A pull request belongs to a branch: without one there is none to show.
	if git, ok := v.Facts.Git.Get(); !ok || git.Branch == "" {
		return nil
	}
	pull, ok := v.Facts.Pull.Get()
	if !ok || pull.Number <= 0 {
		return sessionPullChips(v)
	}
	c := chip(model.TonePlain, "#"+strconv.Itoa(pull.Number))
	if pull.Draft {
		c = append(c, space(), text(model.ToneMuted, "draft"))
	}
	switch {
	case pull.Failed > 0:
		c = append(c, space(), text(model.ToneDanger, "CI ❌ "+strconv.Itoa(pull.Failed)))
	case pull.Pending > 0:
		c = append(c, space(), text(model.ToneCaution, "CI ⏳ "+strconv.Itoa(pull.Pending)))
	case pull.Passed > 0:
		c = append(c, space(), text(model.ToneGood, "CI ✅ "+strconv.Itoa(pull.Passed)))
	}
	if pull.CIDuration > 0 {
		c = append(c, text(model.ToneMuted, " "+span(pull.CIDuration)))
	}
	switch pull.Review {
	case model.ReviewApproved:
		c = append(c, space(), text(model.ToneGood, "approved"))
	case model.ReviewChangesRequested:
		c = append(c, space(), text(model.ToneDanger, "changes"))
	case model.ReviewRequired:
		c = append(c, space(), text(model.ToneCaution, "review"))
	}
	chips := []model.Chip{c}
	if pull.Conflicts {
		chips = append(chips, chip(model.ToneDanger, "conflicts"))
	}
	if pull.Rounds > 0 {
		// Changes asked for after the work was offered as done: rework.
		tone := model.ToneMuted
		if pull.Rounds > 1 {
			tone = model.ToneCaution
		}
		chips = append(chips, chip(tone, "Rounds ×"+strconv.Itoa(pull.Rounds)))
	}
	if size := pull.Size(); size > 0 {
		chips = append(chips, model.Chip{
			text(model.ToneMuted, "Size "),
			text(reviewTone(size), "+"+grouped(float64(pull.Additions))+" -"+grouped(float64(pull.Deletions))),
			text(model.ToneMuted, " "+strconv.Itoa(pull.Files)+"f"),
		})
	}
	if !pull.Created.IsZero() {
		chips = append(chips, chip(model.ToneMuted, "Age "+span(v.Now.Sub(pull.Created))))
	}
	return chips
}

// sessionPullChips shows the pull request Claude Code found, when the GitHub
// CLI gave none (not installed, not signed in, or a GitLab merge request).
func sessionPullChips(v *View) []model.Chip {
	pr, ok := v.Session.PR.Get()
	if !ok {
		return nil
	}
	mark := "#"
	if pr.MergeRequest {
		mark = "!"
	}
	c := chip(model.TonePlain, mark+strconv.Itoa(pr.Number))
	switch pr.ReviewState {
	case "approved":
		c = append(c, space(), text(model.ToneGood, "approved"))
	case "changes_requested":
		c = append(c, space(), text(model.ToneDanger, "changes"))
	case "pending":
		c = append(c, space(), text(model.ToneCaution, "review"))
	case "draft":
		c = append(c, space(), text(model.ToneMuted, "draft"))
	}
	return []model.Chip{c}
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
	if t, ok := v.Facts.Transcript.Get(); ok && t.PermissionMode != "" {
		chips = append(chips, chip(permissionTone(t.PermissionMode), "Perm "+t.PermissionMode))
	}
	if s.Agent != "" {
		chips = append(chips, chip(model.ToneAccent, "Agent "+truncate(s.Agent, sessionCharacters)))
	}
	if s.Vim != "" {
		chips = append(chips, chip(model.ToneNote, "Vim "+s.Vim))
	}
	if moved := movedFrom(&s); moved != "" {
		chips = append(chips, chip(model.ToneCaution, "cd "+truncate(moved, sessionCharacters)))
	}
	if s.AddedDirs > 0 {
		chips = append(chips, chip(model.ToneMuted, "+"+strconv.Itoa(s.AddedDirs)+" dirs"))
	}
	return chips
}

// permissionTone colours a permission mode by how much it lets the agent do
// without asking: bypassing every check is a danger, accepting edits or the
// classifier's auto mode a caution.
func permissionTone(mode string) model.Tone {
	switch mode {
	case "bypassPermissions":
		return model.ToneDanger
	case "acceptEdits", "auto":
		return model.ToneCaution
	default:
		return model.ToneMuted
	}
}

// movedFrom returns where the session works now, relative to where it was
// started, when the two differ: a session that moved works on other files than
// its name suggests. "" when it has not moved or either place is unknown.
func movedFrom(s *model.Session) string {
	if s.ProjectDir == "" || s.Dir == "" || s.Dir == s.ProjectDir {
		return ""
	}
	if rel, ok := strings.CutPrefix(s.Dir, strings.TrimSuffix(s.ProjectDir, "/")+"/"); ok {
		return rel
	}
	return s.Dir
}
