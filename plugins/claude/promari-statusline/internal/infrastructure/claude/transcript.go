package claude

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
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
	if since.Cursor.Format != model.TranscriptFormat {
		since = model.Transcript{}
	}
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
	read.Cursor.Format = model.TranscriptFormat
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
	if at, err := time.Parse(time.RFC3339Nano, jsonx.Or[string](entry, "timestamp")); err == nil {
		t.Stamped(at)
	}
	switch jsonx.Or[string](entry, "type") {
	case entryAssistant:
		readResponse(t, entry)
	case entryUser:
		readPrompt(t, entry)
	case entryPermission:
		t.PermissionChanged(jsonx.Or[string](entry, "permissionMode"))
	case entryAttachment:
		t.Attached(attachmentKind(jsonx.Or[string](jsonx.Child(entry, "attachment"), "type")))
	case entrySystem:
		switch jsonx.Or[string](entry, "subtype") {
		case turnDuration:
			if ms, ok := jsonx.Get[float64](entry, "durationMs"); ok && ms > 0 {
				t.AddTurn(time.Duration(ms * float64(time.Millisecond)))
			}
		case compactBoundary:
			at, _ := time.Parse(time.RFC3339Nano, jsonx.Or[string](entry, "timestamp"))
			t.Compacted(at)
		}
	}
}

// attachmentKind names what Claude Code attached to the conversation: the
// result of a hook, a prompt queued while the agent worked, the diagnostics of
// the editor.
func attachmentKind(kind string) model.AttachmentKind {
	switch kind {
	case "hook_success":
		return model.AttachedHook
	case "hook_non_blocking_error", "hook_blocking_error", "hook_cancelled":
		return model.AttachedHookError
	case "queued_command":
		return model.AttachedQueued
	case "diagnostics":
		return model.AttachedDiagnostics
	}
	return model.AttachedOther
}

// syntheticModel is the model Claude Code names for a response it made up
// itself; no model answered it.
const syntheticModel = "<synthetic>"

// readResponse adds a response of the model. Claude Code writes a response as
// one entry per content block, each repeating its usage, so the response is
// counted on the first entry of each message only (Cursor.Counted). The calls
// and texts are read from every entry: each holds a block of its own.
func readResponse(t *model.Transcript, entry jsonx.Object) {
	message := jsonx.Child(entry, "message")
	side := jsonx.Or[bool](entry, "isSidechain")
	for _, block := range jsonx.Or[[]jsonx.Object](message, "content") {
		switch jsonx.Or[string](block, "type") {
		case "tool_use":
			readCall(t, block, side, jsonx.Or[string](entry, "timestamp"))
		case "text":
			t.Said(jsonx.Or[string](block, "text"), side)
		}
	}
	if t.Cursor.Counted(jsonx.Or[string](message, "id")) {
		return
	}
	r := model.Response{Model: jsonx.Or[string](message, "model"), Side: side, ThinkingMs: jsonx.Or[float64](entry, "thinkingDurationMs")}
	if r.Model == syntheticModel {
		r.Model = ""
	}
	switch jsonx.Or[string](message, "stop_reason") {
	case stopRefusal:
		r.StopReason = model.StoppedRefusal
	case stopMaxTokens:
		r.StopReason = model.StoppedTruncated
	}
	if usage, ok := jsonx.Get[jsonx.Object](message, "usage"); ok {
		server := jsonx.Child(usage, "server_tool_use")
		r.Usage = model.Some(model.Usage{
			Input:        jsonx.Or[float64](usage, "input_tokens"),
			CacheWrite:   jsonx.Or[float64](usage, "cache_creation_input_tokens"),
			CacheWrite1h: jsonx.Or[float64](jsonx.Child(usage, "cache_creation"), "ephemeral_1h_input_tokens"),
			CacheRead:    jsonx.Or[float64](usage, "cache_read_input_tokens"),
			Output:       jsonx.Or[float64](usage, "output_tokens"),
			Thinking:     jsonx.Or[float64](jsonx.Child(usage, "output_tokens_details"), "thinking_tokens"),
			WebSearches:  int(jsonx.Or[float64](server, "web_search_requests")),
			WebFetches:   int(jsonx.Or[float64](server, "web_fetch_requests")),
		})
	}
	t.Responded(r)
}

// readCall reads a tool call into the model's terms: its kind, the file an
// edit names and the command a shell call runs. What the call means is the
// model's to record.
func readCall(t *model.Transcript, block jsonx.Object, side bool, timestamp string) {
	name := jsonx.Or[string](block, "name")
	input := jsonx.Child(block, "input")
	at, _ := time.Parse(time.RFC3339Nano, timestamp)
	call := model.ToolCall{Name: name, At: at, ID: jsonx.Or[string](block, "id"), Kind: callKind(name), Signature: signature(name, block["input"]), Side: side}
	if member, ok := editTools[name]; ok {
		call.File = jsonx.Or[string](input, member)
	} else if name == bashTool {
		call.Command = jsonx.Or[string](input, "command")
	}
	t.Called(call)
}

// searchTools are the tools that search the files.
var searchTools = []string{"Grep", "Glob", "LS"}

// callKind tells reads, searches and edits apart for the trace.
func callKind(name string) int {
	switch {
	case name == "Read":
		return model.CallRead
	case slices.Contains(searchTools, name):
		return model.CallSearch
	}
	if _, edit := editTools[name]; edit {
		return model.CallEdit
	}
	return model.CallOther
}

// bashTool is the tool that runs shell commands.
const bashTool = "Bash"

// signature names a tool call by its tool and input as written, short enough
// to keep.
func signature(name string, input []byte) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	_, _ = h.Write(input)
	return strconv.FormatUint(h.Sum64(), 36)
}

// readResults reads the results of tool calls into the model's terms. A
// refused or interrupted call did not run; the model leaves it undecided.
func readResults(t *model.Transcript, entry jsonx.Object) {
	side := jsonx.Or[bool](entry, "isSidechain")
	for _, block := range jsonx.Or[[]jsonx.Object](jsonx.Child(entry, "message"), "content") {
		if jsonx.Or[string](block, "type") == "tool_result" {
			t.ToolOutput(len(resultText(block)), side)
		}
	}
	result := jsonx.Child(entry, "toolUseResult")
	skipped := jsonx.Or[string](entry, "toolDenialKind") != "" || jsonx.Or[bool](result, "interrupted")
	_, structured := jsonx.Get[jsonx.Object](entry, "toolUseResult")
	at, _ := time.Parse(time.RFC3339Nano, jsonx.Or[string](entry, "timestamp"))
	for _, block := range jsonx.Or[[]jsonx.Object](jsonx.Child(entry, "message"), "content") {
		if jsonx.Or[string](block, "type") != "tool_result" {
			continue
		}
		output := jsonx.Or[string](result, "stdout") + "\n" + jsonx.Or[string](result, "stderr")
		if !structured {
			output = resultText(block)
		}
		t.Resulted(model.ToolResult{
			Side: side, ID: jsonx.Or[string](block, "tool_use_id"), Skipped: skipped, Failed: jsonx.Or[bool](block, "is_error"),
			Output: output, At: at,
		})
	}
}

// resultText returns the text of a tool result: a string, or a list of text blocks.
func resultText(block jsonx.Object) string {
	if s, ok := jsonx.Get[string](block, "content"); ok {
		return s
	}
	var b strings.Builder
	for _, part := range jsonx.Or[[]jsonx.Object](block, "content") {
		b.WriteString(jsonx.Or[string](part, "text"))
		b.WriteByte('\n')
	}
	return b.String()
}

// readPrompt adds a user entry: a prompt a human typed, an interrupted
// response or a refused tool call. Tool results, reminders and notifications
// also arrive as user entries and are not prompts.
func readPrompt(t *model.Transcript, entry jsonx.Object) {
	readResults(t, entry)
	t.Prompted(model.PromptEntry{
		// The note of the interruption is written as a user entry; it is not a prompt.
		Interrupted: jsonx.Or[string](entry, "interruptedMessageId") != "",
		Denied:      jsonx.Or[string](entry, "toolDenialKind") != "",
		Human:       isHumanPrompt(entry),
	})
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
