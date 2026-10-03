// Package claude reads and writes what belongs to Claude Code: its settings,
// the files it keeps for a session, the account that is signed in, its
// releases and the status page of its API.
package claude

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
	"promari-statusline/pkg/jsonx"
)

// ConfigDir returns Claude Code's configuration directory: $CLAUDE_CONFIG_DIR,
// or ~/.claude.
func ConfigDir(sys platform.System) string {
	if dir := sys.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(sys.HomeDir(), ".claude")
}

// topTools is how many tools the status line names.
const topTools = 3

// Transcript counts the tool calls recorded in a session's transcript.
type Transcript struct {
	Sys platform.System
}

var _ repository.ToolStatsReader = Transcript{}

// ToolStats implements repository.ToolStatsReader.
func (t Transcript) ToolStats(_ context.Context, transcript string) (model.ToolStats, error) {
	data, err := t.Sys.ReadFile(transcript)
	if err != nil {
		return model.ToolStats{}, fmt.Errorf("read the transcript: %w", err)
	}
	stats := countTools(data)
	if stats.Total == 0 {
		return model.ToolStats{}, repository.ErrNone
	}
	return stats, nil
}

// The marks of a tool call and of a failed result in a transcript.
const (
	toolUseMark   = `"type":"tool_use"`
	toolNameKey   = `"name":"`
	toolInputKey  = `"input":`
	toolErrorMark = `"is_error":true`
	// toolNameWindow is how far behind the mark of a tool call its name is
	// looked for: the name follows the type and the id of the call.
	toolNameWindow = 200
	// toolNameMax is the longest name taken for a tool.
	toolNameMax = 128
)

// countTools scans a transcript without decoding it: a transcript grows to
// tens of megabytes, and all that is needed is the name of each tool call and
// how many lines carry a failed result.
//
// Only a name that belongs to a tool call is counted. A transcript holds many
// other "name" members (a git remote is {"name":"origin"}), and counting those
// made a tool of every one of them. Text that quotes a tool call is not a
// call either: inside a JSON string its quotes are escaped.
func countTools(transcript []byte) model.ToolStats {
	counts := map[string]int{}
	var order []string // first-seen order, which breaks ties between equal counts
	for rest := transcript; ; {
		_, after, found := bytes.Cut(rest, []byte(toolUseMark))
		if !found {
			break
		}
		rest = after
		name, ok := toolName(after)
		if !ok {
			continue
		}
		if counts[name] == 0 {
			order = append(order, name)
		}
		counts[name]++
	}
	stats := model.ToolStats{}
	for _, name := range order {
		stats.Total += counts[name]
		stats.Top = append(stats.Top, model.ToolCount{Name: name, Count: counts[name]})
	}
	slices.SortStableFunc(stats.Top, func(a, b model.ToolCount) int { return b.Count - a.Count })
	stats.Top = stats.Top[:min(len(stats.Top), topTools)]
	for line := range bytes.Lines(transcript) {
		if bytes.Contains(line, []byte(toolErrorMark)) {
			stats.Errors++
		}
	}
	return stats
}

// toolName returns the name of the tool call whose mark stands before call.
// The name is looked for up to the input of the call, so that a member "name"
// of the input is not taken for it.
func toolName(call []byte) (string, bool) {
	head := call[:min(len(call), toolNameWindow)]
	if before, _, found := bytes.Cut(head, []byte(toolInputKey)); found {
		head = before
	}
	_, value, found := bytes.Cut(head, []byte(toolNameKey))
	if !found {
		return "", false
	}
	end := bytes.IndexByte(value, '"')
	if end <= 0 || end > toolNameMax || bytes.ContainsFunc(value[:end], func(r rune) bool { return !isNameRune(r) }) {
		return "", false
	}
	return string(value[:end]), true
}

// isNameRune reports whether r can be part of a tool's name. The tools of an
// MCP server carry its name, with digits, dots and hyphens (mcp__notion__API-post-page).
func isNameRune(r rune) bool {
	return r == '_' || r == '-' || r == '.' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}

// Todos reads the to-do list Claude Code keeps for a session.
type Todos struct {
	Sys platform.System
}

var _ repository.TodoReader = Todos{}

// Todos implements repository.TodoReader. sessionKey must be a session's Key:
// it becomes part of a file name pattern.
func (t Todos) Todos(_ context.Context, sessionKey string) (model.Todos, error) {
	var newest string
	var modified time.Time
	for _, candidate := range t.Sys.Glob(filepath.Join(ConfigDir(t.Sys), "todos", sessionKey+"*.json")) {
		if at, err := t.Sys.ModTime(candidate); err == nil && (newest == "" || at.After(modified)) {
			newest, modified = candidate, at
		}
	}
	if newest == "" {
		return model.Todos{}, repository.ErrNone
	}
	data, err := t.Sys.ReadFile(newest)
	if err != nil {
		return model.Todos{}, fmt.Errorf("read the to-do list: %w", err)
	}
	// The list is wrapped in an object so that each item is decoded on its own.
	wrapped, _ := jsonx.Parse(slices.Concat([]byte(`{"items":`), data, []byte(`}`)))
	items := jsonx.Or[[]jsonx.Object](wrapped, "items")
	if len(items) == 0 {
		return model.Todos{}, repository.ErrNone
	}
	todos := model.Todos{Total: len(items)}
	for _, item := range items {
		switch jsonx.Or[string](item, "status") {
		case "completed":
			todos.Done++
		case "in_progress":
			if todos.Doing == "" {
				todos.Doing = jsonx.Or[string](item, "content")
			}
		}
	}
	return todos, nil
}

// Account reads the account signed in to the session's configuration: the
// login is kept per configuration directory, so every session started with the
// same CLAUDE_CONFIG_DIR (or without one) runs as the same account.
type Account struct {
	Sys platform.System
}

var _ repository.AccountReader = Account{}

// File returns the file that holds the login: .claude.json in CLAUDE_CONFIG_DIR
// when it is set, and in the home directory otherwise.
func (a Account) File() string {
	if dir := a.Sys.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json")
	}
	return filepath.Join(a.Sys.HomeDir(), ".claude.json")
}

// Account implements repository.AccountReader.
func (a Account) Account(_ context.Context) (string, error) {
	data, err := a.Sys.ReadFile(a.File())
	if err != nil {
		return "", fmt.Errorf("read the account: %w", err)
	}
	config, _ := jsonx.Parse(data)
	email := jsonx.Or[string](jsonx.Child(config, "oauthAccount"), "emailAddress")
	if email == "" {
		return "", repository.ErrNone
	}
	return email, nil
}
