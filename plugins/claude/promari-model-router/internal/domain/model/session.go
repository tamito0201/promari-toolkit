package model

import (
	"slices"
	"time"
)

// SessionSource says where the session model was observed.
type SessionSource string

// Sources of the session model, most reliable first.
const (
	SourceTranscript   SessionSource = "transcript"    // what actually ran
	SourceSessionState SessionSource = "session-state" // SessionStart / PostModelSwitch
	SourceEnv          SessionSource = "ANTHROPIC_MODEL"
	SourceSettings     SessionSource = "settings"
	SourceUnknown      SessionSource = "unknown"
	SourceExplicit     SessionSource = "explicit" // given by a caller (CLI, tests)
)

// Certain reports whether the source is reliable enough to cap a route.
// ANTHROPIC_MODEL and settings may be stale: --model and /model outrank them.
func (s SessionSource) Certain() bool {
	return slices.Contains([]SessionSource{SourceTranscript, SourceSessionState, SourceExplicit}, s)
}

// SessionModel is the observed model of the main conversation.
type SessionModel struct {
	Model  string
	Source SessionSource
}

// Tier is the tier to cap routes with: unknown unless the source is certain.
func (s SessionModel) Tier() Tier {
	if !s.Source.Certain() {
		return TierUnknown
	}
	return TierOf(s.Model)
}

// Session is the aggregate root for one Claude Code session.
type Session struct {
	ID        string
	Model     string
	Source    SessionSource
	UpdatedAt time.Time
}

// NewSession starts the aggregate for a session id (no model observed yet).
func NewSession(id string) Session { return Session{ID: id} }

// SwitchModel returns the session after a model change.
func (s Session) SwitchModel(model string, source SessionSource, at time.Time) Session {
	s.Model, s.Source, s.UpdatedAt = model, source, at
	return s
}
