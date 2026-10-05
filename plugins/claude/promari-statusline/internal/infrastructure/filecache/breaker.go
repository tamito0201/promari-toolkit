package filecache

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
)

// A circuit breaker for a source that is asked on every render and not
// remembered: a source that keeps failing (a tool that hangs until its
// timeout, one that is not installed) would otherwise make every render wait
// for it, and the slowest source sets the time of the whole render.
//
// The status line is a new process on every render, so the breaker's state is
// a file: closed while the source answers; open after breakerTrips failures in
// a row, when the source is not asked; half-open once the wait has passed, when
// one render asks it again. The wait doubles with every failure past the trip,
// from breakerWait up to breakerMaxWait.
const (
	breakerTrips   = 2
	breakerWait    = 30 * time.Second
	breakerMaxWait = 10 * time.Minute
	breakerDir     = "breakers"
)

// ErrOpen is returned instead of asking a source whose breaker is open.
var ErrOpen = errors.New("not asked: the source failed on the last renders")

// breaker is the state of one source's breaker.
type breaker struct {
	Failures int       `json:"failures,omitzero"`
	Last     time.Time `json:"last,omitzero"`
}

// wait is how long the breaker stays open after the last failure.
func (b breaker) wait() time.Duration {
	wait := breakerWait
	for range b.Failures - breakerTrips {
		if wait >= breakerMaxWait {
			break
		}
		wait *= 2
	}
	return min(wait, breakerMaxWait)
}

// Guard asks a source through its breaker. "Nothing to report" is an answer,
// not a failure; a render that was stopped says nothing about the source and
// leaves the breaker as it was.
func Guard[T any](ctx context.Context, s *Store, source string, ask func() (T, error)) (T, error) {
	name := filepath.Join(breakerDir, source+".json")
	state, _ := Load[breaker](s, name)
	now := s.sys.Now()
	if state.Failures >= breakerTrips && now.Sub(state.Last) >= 0 && now.Sub(state.Last) < state.wait() {
		var zero T
		return zero, ErrOpen
	}
	v, err := ask()
	switch {
	case ctx.Err() != nil:
	case err == nil || errors.Is(err, repository.ErrNone):
		if state.Failures > 0 {
			// A breaker that cannot be written asks again next time; nothing worse.
			_ = Save(s, breaker{}, name)
		}
	default:
		_ = Save(s, breaker{Failures: state.Failures + 1, Last: now}, name)
	}
	return v, err
}

// Spends asks for the estimated spending through a breaker: ccusage is run on
// every render, and one that hangs would hold every render up to its timeout.
type Spends struct {
	Store *Store
	Next  repository.SpendReader
}

// Spend implements repository.SpendReader.
func (c Spends) Spend(ctx context.Context, input []byte) (model.Spend, error) {
	return Guard(ctx, c.Store, "spend", func() (model.Spend, error) { return c.Next.Spend(ctx, input) })
}
