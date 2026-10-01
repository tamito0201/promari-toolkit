package model

import "strings"

// Action is what the PreToolUse hook does with one Agent call.
type Action string

// Routing actions.
const (
	ActionNone   Action = "none"   // leave the call as it is
	ActionInject Action = "inject" // rewrite the model
	ActionShadow Action = "shadow" // record what would have been injected
	ActionAsk    Action = "ask"    // ask the user (upgrade above the session)
	ActionSkip   Action = "skip"   // routing disabled
)

// Decision is the immutable routing decision for one Agent call.
type Decision struct {
	Action       Action
	Reason       string
	Target       Tier
	Class        Class
	SubagentType string
	Requested    string
}

// With returns a copy with the given action and reason.
func (d Decision) With(action Action, reason string) Decision {
	d.Action, d.Reason = action, reason
	return d
}

// WithClass returns a copy with the class set.
func (d Decision) WithClass(c Class) Decision {
	d.Class = c
	return d
}

// WithTarget returns a copy with the target set.
func (d Decision) WithTarget(t Tier) Decision {
	d.Target = t
	return d
}

// Rewrites reports whether the decision changes the tool input.
func (d Decision) Rewrites() bool { return d.Action == ActionInject }

// AgentCall is the part of the Agent tool input the router reads.
type AgentCall struct {
	Prompt       string
	SubagentType string
	Model        string
}

// DefaultSubagentType is the type Claude Code runs when an Agent call names none.
const DefaultSubagentType = "general-purpose"

// SubagentTypeOrDefault is the subagent type, defaulting to general-purpose.
func (a AgentCall) SubagentTypeOrDefault() string {
	if a.SubagentType == "" {
		return DefaultSubagentType
	}
	return a.SubagentType
}

// ForceEnv is the Claude Code variable that pins every subagent to one model;
// while it is set the router must not rewrite models.
const ForceEnv = "CLAUDE_CODE_SUBAGENT_MODEL_FORCE"

// SubagentModelForced reports whether ForceEnv is set to a true value (1,
// true, yes or on, in any case and surrounded by spaces). The router, the
// SessionStart notice and `pmr doctor` all ask this one question, so they
// cannot disagree about a value such as "0" or "no".
func SubagentModelForced(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(ForceEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
