// Package state keeps what the status line remembers from one render to the
// next: each session's activity, the rate limits last seen, and a record of
// the last render for diagnosis. Everything lives in the cache directory.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/filecache"
)

const (
	sessionsDir     = "sessions"
	limitsFile      = "rate-limits.json"
	historyFile     = "rate-history.json"
	lastInputFile   = "last-input.json"
	widthFile       = "width.txt"
	sourcesFile     = "sources.json"
	blinkDemoFile   = "blink-demo"
	emptyJSONObject = "{}"
)

// Activities stores each session's activity in a file named after the session
// id, so two sessions never write the same file.
type Activities struct {
	Store *filecache.Store
}

var _ repository.ActivityStore = Activities{}

// sessionsKept is how long the activity of a session that no longer renders is
// kept: a file is written per session, and without this the directory would
// grow by one file for every session ever started.
const sessionsKept = 7 * 24 * time.Hour

// Load implements repository.ActivityStore. A session seen for the first time
// prunes the activities of the sessions that stopped.
func (a Activities) Load(sessionKey string) model.Activity {
	activity, ok := filecache.Load[model.Activity](a.Store, sessionsDir, sessionKey+".json")
	if !ok {
		a.Store.Prune(sessionsDir, sessionsKept)
	}
	return activity
}

// Save implements repository.ActivityStore.
func (a Activities) Save(sessionKey string, activity model.Activity) error {
	return filecache.Save(a.Store, activity, sessionsDir, sessionKey+".json")
}

// remembered is the rate limits as they were last seen.
type remembered struct {
	At     time.Time        `json:"at"`
	Limits model.RateLimits `json:"limits"`
}

// Limits remembers the rate limits across renders and sessions, apart for
// each account: a session of one account never shows another's limits.
type Limits struct {
	Store *filecache.Store
}

var _ repository.RateLimitMemory = Limits{}

// accountsDir holds a directory per account, named after a digest of the
// account so that no address appears in a path.
const accountsDir = "accounts"

// file returns the path parts of an account's file. An unknown account keeps
// the files at the top of the cache, where they were before accounts were told
// apart.
func (Limits) file(account, name string) []string {
	if account == "" {
		return []string{name}
	}
	sum := sha256.Sum256([]byte(account))
	return []string{accountsDir, hex.EncodeToString(sum[:8]), name}
}

// Last implements repository.RateLimitMemory.
func (l Limits) Last(account string) (model.RateLimits, time.Time, error) {
	r, ok := filecache.Load[remembered](l.Store, l.file(account, limitsFile)...)
	if !ok || r.Limits.Empty() {
		return model.RateLimits{}, time.Time{}, repository.ErrNone
	}
	return r.Limits, r.At, nil
}

// Remember implements repository.RateLimitMemory.
func (l Limits) Remember(account string, limits model.RateLimits, at time.Time) error {
	return filecache.Save(l.Store, remembered{At: at, Limits: limits}, l.file(account, limitsFile)...)
}

// History implements repository.RateLimitMemory.
func (l Limits) History(account string) model.RateHistory {
	history, _ := filecache.Load[model.RateHistory](l.Store, l.file(account, historyFile)...)
	return history
}

// SaveHistory implements repository.RateLimitMemory.
func (l Limits) SaveHistory(account string, history model.RateHistory) error {
	return filecache.Save(l.Store, history, l.file(account, historyFile)...)
}

// Recorder keeps the last input and the last measured width. They are the
// first things to look at when a chip is missing or a line is cut: the input
// shows what Claude Code really sent, the width what the layout planned for.
// Every session overwrites them; they describe the most recent render only.
type Recorder struct {
	Store *filecache.Store
}

var _ repository.Recorder = Recorder{}

// Input implements repository.Recorder.
func (r Recorder) Input(raw []byte) {
	if len(raw) == 0 {
		raw = []byte(emptyJSONObject)
	}
	// A record that cannot be written only makes a later diagnosis harder.
	_ = r.Store.WriteRaw(lastInputFile, raw)
}

// Width implements repository.Recorder.
func (r Recorder) Width(source string, budget int) {
	_ = r.Store.WriteRaw(widthFile, []byte(source+" "+strconv.Itoa(budget)+"\n"))
}

// sourceRun is a run as it is written for a person to read. The duration is
// written as text: encoding/json/v2 has no form for a time.Duration and refuses
// to write one, which a recorder that drops its errors would never show.
type sourceRun struct {
	Source  string `json:"source"`
	Took    string `json:"took"`
	Outcome string `json:"outcome"`
}

// outcomes are the names of model.SourceOutcome, in its order.
var outcomes = [...]string{"answered", "none", "failed", "panicked"}

// Sources implements repository.Recorder.
func (r Recorder) Sources(runs []model.SourceRun) {
	written := make([]sourceRun, len(runs))
	for i, run := range runs {
		outcome := "unknown"
		if int(run.Outcome) < len(outcomes) {
			outcome = outcomes[run.Outcome]
		}
		written[i] = sourceRun{Source: run.Source, Took: run.Took.String(), Outcome: outcome}
	}
	_ = filecache.Save(r.Store, written, sourcesFile)
}

// Switches reads the user's on/off choices, each a file in the cache directory.
type Switches struct {
	Store *filecache.Store
}

var _ repository.Switches = Switches{}

// BlinkDemo implements repository.Switches: it is on while the file
// blink-demo exists.
func (s Switches) BlinkDemo() bool { return s.Store.Exists(blinkDemoFile) }
