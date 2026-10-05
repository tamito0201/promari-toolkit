package model

import "time"

// SourceOutcome is how a question to a source of a render ended.
type SourceOutcome uint8

// The outcomes of a question.
const (
	SourceAnswered SourceOutcome = iota // the source answered
	SourceNone                          // it had nothing to report
	SourceFailed                        // it could not answer
	SourcePanicked                      // its reader panicked; the render went on without it
)

// SourceRun is one question a render asked: which source, how long it took
// and how it ended. The runs of the last render show which source holds the
// status line up, or which one is missing and why.
type SourceRun struct {
	Source  string
	Took    time.Duration
	Outcome SourceOutcome
}
