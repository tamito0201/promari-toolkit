// Package claude reads and writes what belongs to Claude Code: its settings,
// the files it keeps for a session, the account that is signed in, its
// releases and the status page of its API.
package claude

import (
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
func (a Account) File() string { return StateFile(a.Sys) }

// StateFile returns Claude Code's state file (.claude.json): in CLAUDE_CONFIG_DIR
// when it is set, and in the home directory otherwise. It holds the login and
// the projects Claude Code has opened.
func StateFile(sys platform.System) string {
	if dir := sys.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json")
	}
	return filepath.Join(sys.HomeDir(), ".claude.json")
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
