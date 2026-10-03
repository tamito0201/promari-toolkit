// Package hook adapts Claude Code hook events (JSON on stdin, JSON on stdout)
// to the application use cases. It is fail-open: any error ends with no
// output and exit status 0, which Claude Code treats as "no decision", so the
// router can never block a prompt or a tool call. Every failure is handed to
// onError, so it is recorded instead of disappearing.
package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
)

// Config is the process-level configuration the adapter reads itself (from
// the defaults and the user file; a project file cannot set it).
type Config struct {
	StdinLimitBytes int64
}

// Handlers are the use cases the adapter dispatches to.
type Handlers struct {
	Config         Config
	SessionStart   usecase.SessionStart
	ModelSwitch    usecase.ModelSwitch
	PromptSubmit   usecase.PromptSubmit
	SubagentStart  usecase.SubagentStart
	SubagentFinish usecase.SubagentFinish
}

// input is the union of the hook event fields the router reads.
type input struct {
	HookEventName  string          `json:"hook_event_name"`
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	Cwd            string          `json:"cwd"`
	Source         string          `json:"source"`
	Model          json.RawMessage `json:"model"`
	FromModel      string          `json:"from_model"`
	ToModel        string          `json:"to_model"`
	Prompt         string          `json:"prompt"`
	ToolName       string          `json:"tool_name"`
	ToolUseID      string          `json:"tool_use_id"`
	AgentID        string          `json:"agent_id"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolResponse   json.RawMessage `json:"tool_response"`
}

type agentInput struct {
	Prompt       string `json:"prompt"`
	SubagentType string `json:"subagent_type"`
	Model        string `json:"model"`
}

type agentResponse struct {
	ResolvedModel   string               `json:"resolvedModel"`
	Status          model.SubagentStatus `json:"status"`
	TotalTokens     int                  `json:"totalTokens"`
	TotalDurationMs int                  `json:"totalDurationMs"`
	Usage           struct {
		InputTokens     int `json:"input_tokens"`
		OutputTokens    int `json:"output_tokens"`
		CacheReadTokens int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

// output is hookSpecificOutput.
type output struct {
	HookSpecificOutput map[string]any `json:"hookSpecificOutput"`
}

func (e input) event() usecase.Event {
	return usecase.Event{SessionID: e.SessionID, Cwd: e.Cwd, TranscriptPath: e.TranscriptPath, ToolUseID: e.ToolUseID, AgentID: e.AgentID}
}

// agentTool reports whether the tool starts a subagent ("Task" is its
// former name). The one place this is decided.
func (e input) agentTool() bool { return e.ToolName == "Agent" || e.ToolName == "Task" }

// decodeField decodes an optional JSON field; an absent field is empty.
func decodeField(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}

// modelName accepts both "model": "id" and "model": {"id": "..."}.
func modelName(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &obj)
	return obj.ID
}

// call is one event being handled.
type call struct {
	h       Handlers
	ctx     context.Context //nolint:containedctx // lives for one event only
	in      input
	emit    func(fields map[string]any)
	onError func(detail string)
}

// eventHandler handles one decoded event.
type eventHandler func(c call)

// events maps hook event names to their handlers; an unknown event does nothing.
var events = map[string]eventHandler{
	"SessionStart":     sessionStart,
	"PostModelSwitch":  modelSwitch,
	"UserPromptSubmit": promptSubmit,
	"PreToolUse":       preToolUse,
	"PostToolUse":      postToolUse,
}

// Run handles one event. It never returns an error to the caller: failures are
// passed to onError(where, detail) and swallowed.
func (h Handlers) Run(ctx context.Context, eventName string, stdin io.Reader, stdout io.Writer, onError func(string, string)) {
	defer func() {
		if r := recover(); r != nil {
			onError(eventName, fmt.Sprintf("panic: %v\n%s", r, debug.Stack()))
		}
	}()
	raw, err := io.ReadAll(io.LimitReader(stdin, h.Config.StdinLimitBytes))
	if err != nil {
		onError(eventName, err.Error())
		return
	}
	var in input
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			onError(eventName, "decode event: "+err.Error())
			return
		}
	}
	handle, ok := events[eventName]
	if !ok {
		return
	}
	handle(call{
		h: h, ctx: ctx, in: in,
		emit: func(fields map[string]any) {
			fields["hookEventName"] = eventName
			if err := json.NewEncoder(stdout).Encode(output{HookSpecificOutput: fields}); err != nil {
				onError(eventName, "encode output: "+err.Error())
			}
		},
		onError: func(detail string) { onError(eventName, detail) },
	})
}

func sessionStart(c call) {
	in := c.in
	if text := c.h.SessionStart.Execute(c.ctx, usecase.SessionStartInput{Event: in.event(), Model: modelName(in.Model), Source: in.Source}); text != "" {
		c.emit(map[string]any{"additionalContext": text})
	}
}

func modelSwitch(c call) { c.h.ModelSwitch.Execute(c.ctx, c.in.event(), c.in.FromModel, c.in.ToModel) }

func promptSubmit(c call) {
	if text := c.h.PromptSubmit.Execute(c.ctx, c.in.event(), c.in.Prompt); text != "" {
		c.emit(map[string]any{"additionalContext": text})
	}
}

func preToolUse(c call) {
	if !c.in.agentTool() {
		return
	}
	var ai agentInput
	if err := decodeField(c.in.ToolInput, &ai); err != nil {
		c.onError("decode tool_input: " + err.Error()) // no output: the call goes through unchanged
		return
	}
	d, _ := c.h.SubagentStart.Execute(c.ctx, c.in.event(), model.AgentCall{Prompt: ai.Prompt, SubagentType: ai.SubagentType, Model: ai.Model})
	switch d.Action {
	case model.ActionInject:
		// updatedInput replaces the whole input object: copy every original
		// field, change only `model`. No permissionDecision — "allow" would
		// skip the user's own permission rules, and the rewrite applies
		// without it (verified on Claude Code 2.1.285).
		var full map[string]any
		if json.Unmarshal(c.in.ToolInput, &full) != nil || full == nil {
			// The ledger already holds the inject: say it was not applied, or
			// the ledger reads as a rewrite that never happened.
			c.onError("rewrite tool_input: not an object, the model was not changed")
			return
		}
		full["model"] = string(d.Target)
		c.emit(map[string]any{"updatedInput": full})
	case model.ActionAsk:
		c.emit(map[string]any{
			"permissionDecision":       "ask",
			"permissionDecisionReason": c.h.SubagentStart.AskReason(c.in.Cwd, d.Target),
		})
	default:
		// none / shadow / skip: no output, the call goes through unchanged.
	}
}

func postToolUse(c call) {
	if !c.in.agentTool() {
		return
	}
	var ai agentInput
	var ar agentResponse
	if err := decodeField(c.in.ToolInput, &ai); err != nil {
		c.onError("decode tool_input: " + err.Error())
		return
	}
	if err := decodeField(c.in.ToolResponse, &ar); err != nil {
		c.onError("decode tool_response: " + err.Error())
		return
	}
	c.h.SubagentFinish.Execute(c.ctx, c.in.event(), model.SubagentOutcome{
		ToolUseID:    c.in.ToolUseID,
		SubagentType: model.AgentCall{SubagentType: ai.SubagentType}.SubagentTypeOrDefault(), Requested: ai.Model,
		Resolved: ar.ResolvedModel, Status: ar.Status, TotalTokens: ar.TotalTokens, InputTokens: ar.Usage.InputTokens,
		OutputTokens: ar.Usage.OutputTokens, CacheReadTokens: ar.Usage.CacheReadTokens, DurationMS: ar.TotalDurationMs,
	}, ai.Prompt)
}
