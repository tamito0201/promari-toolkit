// Package state keeps what the status line remembers from one render to the
// next: each session's activity, the rate limits last seen, and a record of
// the last render for diagnosis. Everything lives in the cache directory.
package state

import (
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
	blinkDemoFile   = "blink-demo"
	emptyJSONObject = "{}"
)

// Activities stores each session's activity in a file named after the session
// id, so two sessions never write the same file.
type Activities struct {
	Store *filecache.Store
}

var _ repository.ActivityStore = Activities{}

// Load implements repository.ActivityStore.
func (a Activities) Load(sessionKey string) model.Activity {
	activity, _ := filecache.Load[model.Activity](a.Store, sessionsDir, sessionKey+".json")
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

// Limits remembers the rate limits across renders and sessions.
type Limits struct {
	Store *filecache.Store
}

var _ repository.RateLimitMemory = Limits{}

// Last implements repository.RateLimitMemory.
func (l Limits) Last() (model.RateLimits, time.Time, error) {
	r, ok := filecache.Load[remembered](l.Store, limitsFile)
	if !ok || r.Limits.Empty() {
		return model.RateLimits{}, time.Time{}, repository.ErrNone
	}
	return r.Limits, r.At, nil
}

// Remember implements repository.RateLimitMemory.
func (l Limits) Remember(limits model.RateLimits, at time.Time) error {
	return filecache.Save(l.Store, remembered{At: at, Limits: limits}, limitsFile)
}

// History implements repository.RateLimitMemory.
func (l Limits) History() model.RateHistory {
	history, _ := filecache.Load[model.RateHistory](l.Store, historyFile)
	return history
}

// SaveHistory implements repository.RateLimitMemory.
func (l Limits) SaveHistory(history model.RateHistory) error {
	return filecache.Save(l.Store, history, historyFile)
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

// Switches reads the user's on/off choices, each a file in the cache directory.
type Switches struct {
	Store *filecache.Store
}

var _ repository.Switches = Switches{}

// BlinkDemo implements repository.Switches: it is on while the file
// blink-demo exists.
func (s Switches) BlinkDemo() bool { return s.Store.Exists(blinkDemoFile) }
