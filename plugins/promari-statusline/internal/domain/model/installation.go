package model

// StatusLineSetting is the statusLine entry of Claude Code's settings.
type StatusLineSetting struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Padding int    `json:"padding"`
}

// CommandType is the only statusLine type Claude Code runs as a process.
const CommandType = "command"

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
