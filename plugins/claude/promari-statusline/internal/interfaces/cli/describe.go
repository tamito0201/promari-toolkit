package cli

import (
	"strconv"
	"strings"

	"promari-statusline/internal/application/usecase"
)

// projectsNamed is how many projects that show another status line are named
// in the doctor's line; the rest are counted.
const projectsNamed = 5

// toolHint is what the doctor says about an optional tool that is missing: the
// chips it hides and how to get it.
type toolHint struct {
	chips   string
	install string
}

// toolHints are the words for the tools of usecase.OptionalTools.
func toolHints() map[string]toolHint {
	return map[string]toolHint{
		"git":            {"🌿 Git", "https://git-scm.com/downloads"},
		"gh":             {"🔀 PR", "https://cli.github.com"},
		"ccusage":        {"Today, Blk, $/h and Est", "npm install -g ccusage"},
		"nowplaying-cli": {"🎵 Music", "brew install nowplaying-cli (macOS)"},
	}
}

// describe words one check of the doctor: the name of what was checked, and
// what was found with what to run about it.
func describe(c *usecase.Check) (name, detail string) {
	switch c.Finding {
	case usecase.SettingsRunsThis:
		return "settings " + c.Subject, "statusLine runs " + c.Command
	case usecase.SettingsMissing:
		return "settings " + c.Subject, "no statusLine; run `psl setup`"
	case usecase.SettingsUnreadable:
		return "settings " + c.Subject, "cannot be read: " + c.Err.Error()
	case usecase.SettingsRunOther:
		return "settings " + c.Subject, "statusLine runs another command: " + c.Command + "; run `psl setup` to use this plugin"
	case usecase.SettingsNoRefresh:
		return "settings " + c.Subject, "statusLine runs " + c.Command +
			" but has no refreshInterval: an idle session will not follow the other sessions; run `psl setup`"
	case usecase.ProjectsShowThis:
		return c.Subject, strconv.Itoa(c.Projects) + " projects show this status line"
	case usecase.ProjectsShowOther:
		named := c.Others[:min(len(c.Others), projectsNamed)]
		more := ""
		if len(c.Others) > len(named) {
			more = " and " + strconv.Itoa(len(c.Others)-len(named)) + " more"
		}
		return c.Subject, strconv.Itoa(len(c.Others)) + " of " + strconv.Itoa(c.Projects) + " projects show another status line (" +
			strings.Join(named, ", ") + more + "); run `psl setup --global`"
	case usecase.ProjectsUnlisted:
		return c.Subject, "cannot be listed: " + c.Err.Error()
	case usecase.BinaryInSync:
		return "installed binary " + c.Subject, "is the running binary"
	case usecase.BinaryMissing:
		return "installed binary " + c.Subject, "not installed; run `psl setup`"
	case usecase.BinaryUnreadable:
		return "installed binary " + c.Subject, "cannot be compared: " + c.Err.Error()
	case usecase.BinaryStale:
		return "installed binary " + c.Subject, "differs from the running binary; the next session start refreshes it, or run `psl setup`"
	case usecase.LauncherFailed:
		return c.Subject, c.Detail
	case usecase.LauncherUnreadable:
		return c.Subject, "its record cannot be read: " + c.Err.Error()
	case usecase.ToolFound:
		return c.Subject, c.Detail
	case usecase.ToolMissing:
		hint, ok := toolHints()[c.Subject]
		if !ok {
			return c.Subject, "not found"
		}
		return c.Subject, "not found; " + hint.chips + " will not be shown (install: " + hint.install + ")"
	case usecase.TerminalMeasured:
		return c.Subject, strconv.Itoa(c.Cells) + " cells (" + c.Source + "), " + strconv.Itoa(c.Budget) + " used per line"
	}
	return c.Subject, ""
}
