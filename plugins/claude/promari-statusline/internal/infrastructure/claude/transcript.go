package claude

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
	"promari-statusline/pkg/jsonx"
)

// Transcript reads a session's transcript.
type Transcript struct {
	Sys platform.System
}

var _ repository.TranscriptReader = Transcript{}

// Transcript implements repository.TranscriptReader. It reads only what was
// written after since.Cursor: a transcript grows to tens of megabytes and only
// ever grows. A file shorter than the cursor is a new transcript and is read
// from the start. A line not yet finished is left for the next read.
func (t Transcript) Transcript(_ context.Context, path string, since model.Transcript) (model.Transcript, error) {
	data, size, err := t.Sys.ReadFrom(path, since.Cursor.Offset)
	if err != nil {
		return model.Transcript{}, fmt.Errorf("read the transcript: %w", err)
	}
	read := since
	if size < since.Cursor.Offset {
		read = model.Transcript{}
		if data, _, err = t.Sys.ReadFrom(path, 0); err != nil {
			return model.Transcript{}, fmt.Errorf("read the transcript: %w", err)
		}
	}
	end := bytes.LastIndexByte(data, '\n') + 1
	complete := data[:end]
	read.CountTools(countTools(complete))
	for line := range bytes.Lines(complete) {
		readEntry(&read, line)
	}
	read.Cursor.Offset += int64(end)
	if read.Requests == 0 && read.Tools.Total == 0 && read.Prompts == 0 {
		return read, repository.ErrNone
	}
	return read, nil
}

// The kinds of entries a transcript holds, and their kinds of system entries.
const (
	entryAssistant  = "assistant"
	entryUser       = "user"
	entrySystem     = "system"
	entryAttachment = "attachment"
	entryPermission = "permission-mode"
	turnDuration    = "turn_duration"
	compactBoundary = "compact_boundary"
	humanOrigin     = "human"
	// The stop reasons counted.
	stopRefusal   = "refusal"
	stopMaxTokens = "max_tokens"
)

// editTools are the tools that change a file, with the member of their input
// that names it.
var editTools = map[string]string{
	"Edit": "file_path", "MultiEdit": "file_path", "Write": "file_path", "NotebookEdit": "notebook_path",
}

// readEntry adds one line of the transcript. A line that is not a JSON object
// is skipped: the transcript belongs to Claude Code and its format grows.
func readEntry(t *model.Transcript, line []byte) {
	entry, ok := jsonx.Parse(line)
	if !ok {
		return
	}
	if at, err := time.Parse(time.RFC3339Nano, jsonx.Or[string](entry, "timestamp")); err == nil && t.Started.IsZero() {
		t.Started = at
	}
	switch jsonx.Or[string](entry, "type") {
	case entryAssistant:
		readResponse(t, entry)
	case entryUser:
		readPrompt(t, entry)
	case entryPermission:
		if mode := jsonx.Or[string](entry, "permissionMode"); mode != "" {
			t.PermissionMode = mode
		}
	case entryAttachment:
		readAttachment(t, jsonx.Or[string](jsonx.Child(entry, "attachment"), "type"))
	case entrySystem:
		switch jsonx.Or[string](entry, "subtype") {
		case turnDuration:
			if ms, ok := jsonx.Get[float64](entry, "durationMs"); ok && ms > 0 {
				t.AddTurn(time.Duration(ms * float64(time.Millisecond)))
			}
		case compactBoundary:
			t.Compactions++
			if at, err := time.Parse(time.RFC3339Nano, jsonx.Or[string](entry, "timestamp")); err == nil {
				t.LastCompaction = at
			}
		}
	}
}

// readAttachment adds what Claude Code attached to the conversation: the
// result of a hook, a prompt queued while the agent worked, the diagnostics of
// the editor.
func readAttachment(t *model.Transcript, kind string) {
	switch kind {
	case "hook_success":
		t.Hooks++
	case "hook_non_blocking_error", "hook_blocking_error", "hook_cancelled":
		t.Hooks++
		t.HookErrors++
	case "queued_command":
		t.Queued++
	case "diagnostics":
		t.Diagnostics++
	}
}

// readResponse adds a response of the model. Claude Code writes a response as
// one entry per content block, each repeating its usage, so the usage is
// counted on the first entry of each message only (Cursor.Counted). The files edited are read
// from every entry: each holds a block of its own.
func readResponse(t *model.Transcript, entry jsonx.Object) {
	message := jsonx.Child(entry, "message")
	for _, block := range jsonx.Or[[]jsonx.Object](message, "content") {
		if member, ok := editTools[jsonx.Or[string](block, "name")]; ok && jsonx.Or[string](block, "type") == "tool_use" {
			t.AddFile(jsonx.Or[string](jsonx.Child(block, "input"), member))
		}
	}
	if t.Cursor.Counted(jsonx.Or[string](message, "id")) {
		return
	}
	t.Requests++
	if jsonx.Or[bool](entry, "isSidechain") {
		t.SideRequests++
	}
	if name := jsonx.Or[string](message, "model"); name != "" && name != "<synthetic>" {
		if t.Models == nil {
			t.Models = map[string]int{}
		}
		t.Models[name]++
	}
	switch jsonx.Or[string](message, "stop_reason") {
	case stopRefusal:
		t.Refusals++
	case stopMaxTokens:
		t.Truncated++
	}
	t.ThinkingSeconds += jsonx.Or[float64](entry, "thinkingDurationMs") / float64(time.Second/time.Millisecond)
	usage := jsonx.Child(message, "usage")
	tokens := &t.Tokens
	tokens.Input += jsonx.Or[float64](usage, "input_tokens")
	tokens.CacheWrite += jsonx.Or[float64](usage, "cache_creation_input_tokens")
	tokens.CacheWrite1h += jsonx.Or[float64](jsonx.Child(usage, "cache_creation"), "ephemeral_1h_input_tokens")
	tokens.CacheRead += jsonx.Or[float64](usage, "cache_read_input_tokens")
	tokens.Output += jsonx.Or[float64](usage, "output_tokens")
	tokens.Thinking += jsonx.Or[float64](jsonx.Child(usage, "output_tokens_details"), "thinking_tokens")
	server := jsonx.Child(usage, "server_tool_use")
	t.WebSearches += int(jsonx.Or[float64](server, "web_search_requests"))
	t.WebFetches += int(jsonx.Or[float64](server, "web_fetch_requests"))
}

// readPrompt adds a user entry: a prompt a human typed, an interrupted
// response or a refused tool call. Tool results, reminders and notifications
// also arrive as user entries and are not prompts.
func readPrompt(t *model.Transcript, entry jsonx.Object) {
	if jsonx.Or[string](entry, "interruptedMessageId") != "" {
		// The note of the interruption is written as a user entry; it is not a prompt.
		t.Interrupts++
		return
	}
	if jsonx.Or[string](entry, "toolDenialKind") != "" {
		t.Denials++
	}
	if isHumanPrompt(entry) {
		t.Prompts++
	}
}

// isHumanPrompt reports whether a user entry is a prompt a human typed. Claude
// Code names the origin of a prompt; an entry written before it did is a
// prompt when it carries text and is neither a meta entry, a compaction summary
// nor a tool result.
func isHumanPrompt(entry jsonx.Object) bool {
	if _, named := entry["origin"]; named {
		return jsonx.Or[string](jsonx.Child(entry, "origin"), "kind") == humanOrigin
	}
	if jsonx.Or[bool](entry, "isMeta") || jsonx.Or[bool](entry, "isCompactSummary") || jsonx.Or[bool](entry, "isSidechain") {
		return false
	}
	message := jsonx.Child(entry, "message")
	if _, ok := jsonx.Get[string](message, "content"); ok {
		return true
	}
	for _, block := range jsonx.Or[[]jsonx.Object](message, "content") {
		if jsonx.Or[string](block, "type") == "tool_result" {
			return false
		}
	}
	return len(jsonx.Or[[]jsonx.Object](message, "content")) > 0
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

// countTools scans part of a transcript without decoding it, and returns the
// tools called, in the order first called, and how many lines carry a failed
// result.
//
// Only a name that belongs to a tool call is counted. A transcript holds many
// other "name" members (a git remote is {"name":"origin"}), and counting those
// made a tool of every one of them. Text that quotes a tool call is not a
// call either: inside a JSON string its quotes are escaped.
func countTools(transcript []byte) (calls []model.ToolCount, failed int) {
	index := map[string]int{}
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
		if i, seen := index[name]; seen {
			calls[i].Count++
			continue
		}
		index[name] = len(calls)
		calls = append(calls, model.ToolCount{Name: name, Count: 1})
	}
	for line := range bytes.Lines(transcript) {
		if bytes.Contains(line, []byte(toolErrorMark)) {
			failed++
		}
	}
	return calls, failed
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
