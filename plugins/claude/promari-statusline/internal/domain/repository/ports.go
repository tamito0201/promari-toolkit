// Package repository declares the ports through which the status line reaches
// the world outside the process. The use cases depend on these interfaces
// only; the adapters in internal/infrastructure implement them. Each port is
// one source with one question, so a use case names exactly what it needs and
// a test replaces exactly that.
package repository

import (
	"context"
	"errors"
	"time"

	"promari-statusline/internal/domain/model"
)

// ErrNone is returned by a reader that looked and found nothing to report: no
// pull request, no song, no incident. It is an answer, not a failure: a reader
// that could not look returns its own error, wrapped. The one reader that
// returns a value along with ErrNone is TranscriptReader, whose value is where
// the next read continues.
var ErrNone = errors.New("nothing to report")

// Clock tells the time.
type Clock interface {
	Now() time.Time
}

// GitReader reads the state of a working tree. It returns ErrNone outside a
// repository or on a detached HEAD.
type GitReader interface {
	Git(ctx context.Context, dir string) (model.Git, error)
}

// HistoryReader reads what the commits of a branch say.
type HistoryReader interface {
	History(ctx context.Context, dir, branch string) (model.History, error)
}

// PullRequestReader reads the pull request of a branch.
type PullRequestReader interface {
	PullRequest(ctx context.Context, dir, branch string) (model.PullRequest, error)
}

// ReviewQueueReader reads the pull requests of a repository that wait for the
// user's review. It returns ErrNone when none waits.
type ReviewQueueReader interface {
	ReviewQueue(ctx context.Context, dir string) (model.ReviewQueue, error)
}

// WorkloadReader reads the work the user owes in a repository: assigned
// issues, open and finished pull requests, the runs of the default branch. It
// returns ErrNone when none of it can be read.
type WorkloadReader interface {
	Workload(ctx context.Context, dir string) (model.Workload, error)
}

// SpendReader reads the estimated spending. input is the session report as
// Claude Code sent it, which the estimator reads on its standard input.
type SpendReader interface {
	Spend(ctx context.Context, input []byte) (model.Spend, error)
}

// CodexReader reads the usage windows Codex last reported.
type CodexReader interface {
	Codex(ctx context.Context) (model.CodexLimits, error)
}

// TranscriptReader reads a session's transcript. It continues from since, what
// an earlier read of the same transcript returned (the zero Transcript reads it
// from the start), so that a transcript of tens of megabytes is read once.
type TranscriptReader interface {
	Transcript(ctx context.Context, path string, since model.Transcript) (model.Transcript, error)
}

// TodoReader reads the to-do list of a session.
type TodoReader interface {
	Todos(ctx context.Context, sessionKey string) (model.Todos, error)
}

// TrackReader reads the song that is playing.
type TrackReader interface {
	Track(ctx context.Context) (model.Track, error)
}

// IncidentReader reads the API's status page. It returns ErrNone while
// everything is operational.
type IncidentReader interface {
	Incident(ctx context.Context) (model.Incident, error)
}

// ReleaseReader reads the newest released version of Claude Code.
type ReleaseReader interface {
	Latest(ctx context.Context) (string, error)
}

// AccountReader reads the name of the signed-in account.
type AccountReader interface {
	Account(ctx context.Context) (string, error)
}

// MachineReader reads the state of the computer. dir is the directory whose
// disk is measured.
type MachineReader interface {
	Machine(ctx context.Context, dir string) model.Machine
}

// ActivityStore keeps each session's Activity between renders.
type ActivityStore interface {
	// Load returns the stored activity, or a new one for an unknown session.
	Load(sessionKey string) model.Activity
	Save(sessionKey string, a model.Activity) error
}

// RateLimitMemory remembers the rate limits across renders and sessions: the
// first render of a session does not carry them. The limits belong to an
// account, so each account is remembered apart; "" is a session whose account
// is unknown.
type RateLimitMemory interface {
	// Last returns the limits last seen and when. It returns ErrNone when none
	// were remembered.
	Last(account string) (model.RateLimits, time.Time, error)
	Remember(account string, l model.RateLimits, at time.Time) error
	History(account string) model.RateHistory
	SaveHistory(account string, h model.RateHistory) error
}

// UsageBoard shares the plan usage with other tools on this machine. Claude
// Code tells the rate limits to the status line only; a hook is not told, so a
// tool that runs as a hook reads them from the board.
type UsageBoard interface {
	PostClaude(l model.RateLimits, at time.Time) error
	PostCodex(l model.CodexLimits, at time.Time) error
}

// PeerBoard shares each session's summary with the status lines of the other
// sessions on this machine. Every session posts to a file of its own, so two
// sessions never write the same file.
type PeerBoard interface {
	// Post publishes the caller's session, stamped with the Claude Code process
	// that runs it.
	Post(p model.Peer) error
	// Roster returns the sessions still running, the caller's own included. A
	// session whose Claude Code has exited is left out and its file removed.
	Roster() model.Roster
}

// Terminal measures the terminal the status line is drawn in.
type Terminal interface {
	// Width returns the width in cells and where the number came from.
	Width() (cells int, source string)
}

// Recorder keeps what the last render saw, for diagnosis.
type Recorder interface {
	Input(raw []byte)
	Width(source string, budget int)
	// Sources keeps how each question of the render went.
	Sources(runs []model.SourceRun)
}

// Switches are the user's on/off choices outside the settings file.
type Switches interface {
	// BlinkDemo makes every warning blink, to check that blinking works.
	BlinkDemo() bool
}

// SettingsReader reads a Claude Code settings file.
type SettingsReader interface {
	Path() string
	// StatusLine returns the current entry. It returns ErrNone when there is none.
	StatusLine() (model.StatusLineSetting, error)
}

// SettingsStore reads and edits a Claude Code settings file.
type SettingsStore interface {
	SettingsReader
	// SetStatusLine writes the entry and returns the path of the backup it
	// made of the previous file, or "" when there was nothing to back up.
	SetStatusLine(s model.StatusLineSetting) (backup string, err error)
	RemoveStatusLine() (backup string, err error)
}

// ProjectStore reaches the projects Claude Code has opened. A project's
// settings take precedence over the user's, so a project that sets its own
// status line hides the user's on every terminal opened in it.
type ProjectStore interface {
	// Projects returns the directories of the projects Claude Code has opened
	// that still exist, the user's home (whose settings are the user's) left out.
	Projects() ([]string, error)
	// Shared is the project's settings file shared through the repository
	// (.claude/settings.json). It belongs to the repository and its other
	// users, so it can be read and never written.
	Shared(dir string) SettingsReader
	// Local is the project's personal settings file (.claude/settings.local.json),
	// which takes precedence over the shared one.
	Local(dir string) SettingsStore
	// KeepOutOfGit makes sure the personal settings file is not picked up by
	// git: it is ignored already, or it is added to the repository's own
	// exclude list (.git/info/exclude, never shared). added is false when
	// nothing had to change, and for a directory outside a repository. It runs
	// git, which ctx stops.
	KeepOutOfGit(ctx context.Context, dir string) (added bool, err error)
}

// BinaryInspector looks at the copy of the running binary kept at a path that
// does not change between plugin versions, which is what the settings point at.
type BinaryInspector interface {
	// Command returns the command line that runs the installed copy.
	Command() string
	Path() string
	// InSync reports whether the installed copy is the running binary. It
	// returns ErrNone when no copy is installed.
	InSync() (bool, error)
}

// BinaryStore keeps the copy of the running binary.
type BinaryStore interface {
	BinaryInspector
	Install() error
	Remove() error
}

// ToolFinder looks for the optional tools on PATH.
type ToolFinder interface {
	Find(name string) (path string, ok bool)
}

// LauncherLog reads the failure the launcher (bin/psl) last recorded while it
// tried to provide a binary. It returns ErrNone when there is none.
type LauncherLog interface {
	LastError() (string, error)
}
