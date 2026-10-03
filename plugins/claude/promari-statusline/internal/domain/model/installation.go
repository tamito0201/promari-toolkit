package model

// StatusLineSetting is the statusLine entry of Claude Code's settings.
type StatusLineSetting struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Padding int    `json:"padding"`
	// RefreshInterval makes Claude Code run the status line every so many
	// seconds besides on its own events; zero leaves it to the events.
	RefreshInterval int `json:"refreshInterval,omitzero"`
}

// CommandType is the only statusLine type Claude Code runs as a process.
const CommandType = "command"

// RefreshSeconds is how often an idle session's status line is drawn again,
// so that it follows the other sessions: without it a status line is drawn
// only when its own session does something.
const RefreshSeconds = 5

// Check is one line of a diagnosis.
type Check struct {
	Level  CheckLevel
	Name   string
	Detail string
}

// CheckLevel says how a check went.
type CheckLevel uint8

// The outcomes of a check.
const (
	CheckOK CheckLevel = iota
	CheckWarn
	CheckFail
)
