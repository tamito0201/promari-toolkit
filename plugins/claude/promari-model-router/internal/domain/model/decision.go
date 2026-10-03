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

// The reasons a call kept the session's tier although it had a class: a
// guard held it (it would have gone to a cheaper tier), or there was nothing
// to rewrite. The router writes them and the evaluation reads them, so they
// are named once, here.
const (
	ReasonDangerKeep     = "danger-keep"      // the prompt names a dangerous operation
	ReasonRetryKeep      = "retry-keep"       // the same brief was delegated before
	ReasonContextKeep    = "context-keep"     // the brief leans on the conversation
	ReasonRiskHold       = "risk-hold"        // P(the cheaper tier suffices) is below τ
	ReasonLongPromptKeep = "long-prompt-keep" // a long prompt the rules are unsure of
	ReasonPosteriorHold  = "posterior-hold"   // the ledger shows the target tier failing, and no tier above has the evidence to take it
	ReasonGateClosed     = "gate-closed"      // the class's Triage gate is closed
	ReasonUnknownSession = "unknown-session"  // the session's tier is not known
	ReasonSameTier       = "same-tier"        // the target is the session's tier already
)

// held are the reasons of a guard that kept a call off a cheaper tier.
var held = map[string]bool{
	ReasonDangerKeep: true, ReasonRetryKeep: true, ReasonContextKeep: true, ReasonRiskHold: true,
	ReasonLongPromptKeep: true, ReasonPosteriorHold: true, ReasonGateClosed: true,
}

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

// Held reports whether a guard kept the call off the cheaper tier its class
// names.
func (d Decision) Held() bool { return held[d.Reason] }

// Classified reports whether the decision stands for its class: it was routed
// (or would have been, in shadow mode), a guard held it, or its tier needed
// nothing done. Otherwise the router abstained and the class says nothing.
func (d Decision) Classified() bool {
	return d.Rewrites() || d.Action == ActionShadow || d.Held() ||
		d.Reason == ReasonSameTier || d.Reason == ReasonUnknownSession
}

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
