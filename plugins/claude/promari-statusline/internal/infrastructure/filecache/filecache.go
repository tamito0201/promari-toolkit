// Package filecache keeps small JSON files in the status line's cache
// directory, and remembers the answers of slow sources between renders.
//
// The status line is a new process on every render, several times a second
// while a session is busy. Whatever takes longer than a few milliseconds (a
// GitHub lookup, a status page, a scan of a transcript) is asked once and then
// read from here until it is old.
package filecache

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"path/filepath"
	"time"

	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
)

// dirName is the plugin's directory under the user's cache directory.
const dirName = "promari-statusline"

// Store is a directory of JSON files.
type Store struct {
	sys platform.System
	dir string
}

// NewStore returns the store in $XDG_CACHE_HOME/promari-statusline, or in
// ~/.cache/promari-statusline when XDG_CACHE_HOME is not set.
func NewStore(sys platform.System) *Store {
	base := sys.Getenv("XDG_CACHE_HOME")
	if base == "" {
		base = filepath.Join(sys.HomeDir(), ".cache")
	}
	return &Store{sys: sys, dir: filepath.Join(base, dirName)}
}

// Path returns the path of a file in the store.
func (s *Store) Path(name ...string) string {
	return filepath.Join(append([]string{s.dir}, name...)...)
}

// Exists reports whether a file is in the store.
func (s *Store) Exists(name string) bool {
	_, err := s.sys.ModTime(s.Path(name))
	return err == nil
}

// WriteRaw stores bytes as they are.
func (s *Store) WriteRaw(name string, data []byte) error {
	return s.sys.WriteFile(s.Path(name), data, platform.Private)
}

// ReadRaw returns a file of the store as it is, with when it was written.
func (s *Store) ReadRaw(name string) ([]byte, time.Time, error) {
	at, err := s.sys.ModTime(s.Path(name))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("stat %s: %w", s.Path(name), err)
	}
	data, err := s.sys.ReadFile(s.Path(name))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("read %s: %w", s.Path(name), err)
	}
	return data, at, nil
}

// durations writes a time.Duration as its text ("1m30s") and reads it back.
// encoding/json/v2 has no form of its own for a duration and refuses to write
// one: a fact holding a duration would fail its whole save, and a cache that
// drops its errors would then ask its slow source on every render.
var durations = json.JoinOptions(
	json.WithMarshalers(json.MarshalFunc(func(d time.Duration) ([]byte, error) {
		return json.Marshal(d.String())
	})),
	json.WithUnmarshalers(json.UnmarshalFunc(func(data []byte, d *time.Duration) error {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		parsed, err := time.ParseDuration(text)
		if err != nil {
			return fmt.Errorf("read a duration: %w", err)
		}
		*d = parsed
		return nil
	})),
)

// Load reads a file of the store as T. ok is false when the file is missing or
// does not hold a T; the zero value is then returned.
func Load[T any](s *Store, name ...string) (v T, ok bool) {
	data, err := s.sys.ReadFile(s.Path(name...))
	if err != nil {
		return v, false
	}
	if err := json.Unmarshal(data, &v, durations); err != nil {
		var zero T
		return zero, false
	}
	return v, true
}

// Save writes v to a file of the store.
func Save[T any](s *Store, v T, name ...string) error {
	data, err := json.Marshal(v, durations)
	if err != nil {
		return fmt.Errorf("encode %s: %w", s.Path(name...), err)
	}
	return s.sys.WriteFile(s.Path(name...), data, platform.Private)
}

// Prune removes the files of a directory of the store not written for longer
// than age. A file that cannot be removed is left for the next time. Whatever
// writes a file per key (a session, a branch) prunes its directory when a new
// key arrives, so that the directory holds the keys in use and no more.
func (s *Store) Prune(dir string, age time.Duration) {
	now := s.sys.Now()
	// "*.json.tmp*" as well: a render killed mid-write leaves the temporary
	// file of a key that may never be written again, so no later write of the
	// same target would sweep it.
	for _, pattern := range []string{"*.json", "*.json.tmp*"} {
		for _, path := range s.sys.Glob(s.Path(dir, pattern)) {
			if at, err := s.sys.ModTime(path); err == nil && now.Sub(at) > age {
				_ = s.sys.Remove(path)
			}
		}
	}
}

// entry is one remembered answer. Data is stored, never rendered text: a
// stored chip would freeze its blink and every countdown for as long as the
// entry lives.
type entry[T any] struct {
	At  time.Time `json:"at"`
	Key string    `json:"key,omitzero"`
	// Found is false for an answer of "nothing" or a failure. Both are
	// remembered, so that a branch without a pull request, or a tool that is
	// not installed, is not asked about on every render.
	Found bool `json:"found"`
	Value T    `json:"value"`
}

// memosKept is how long the remembered answer for a key that is no longer
// asked about (a branch merged and deleted, a repository closed) is kept.
const memosKept = 7 * 24 * time.Hour

// Memo returns the answer remembered for key while it is younger than ttl, and
// otherwise asks and remembers the answer. A remembered "nothing" is returned
// as repository.ErrNone.
//
// Each key is remembered in a file of its own under the directory kind:
// sessions running side by side in other repositories or on other branches
// would otherwise replace each other's answer on every render and ask the slow
// source every time. An empty key is the one answer of kind, kept in kind.json.
//
// An answer cut short because ctx ended (the render was stopped) is returned
// but not remembered: it says nothing about the source, and remembering it
// would hide the source's real answer for as long as ttl.
func Memo[T any](ctx context.Context, s *Store, kind, key string, ttl time.Duration, ask func() (T, error)) (T, error) {
	now := s.sys.Now()
	name := kind + ".json"
	if key != "" {
		name = filepath.Join(kind, digest(key)+".json")
	}
	e, known := Load[entry[T]](s, name)
	// The key is compared as well as its digest: two keys that share a digest
	// must not answer for each other.
	if known && e.Key == key && fresh(e.At, now, ttl) {
		if !e.Found {
			var zero T
			return zero, repository.ErrNone
		}
		return e.Value, nil
	}
	if !known && key != "" {
		s.Prune(kind, memosKept)
	}
	v, err := ask()
	if ctx.Err() != nil {
		return v, err
	}
	// A cache that cannot be written costs a repeated question on the next
	// render; the answer is still good.
	_ = Save(s, entry[T]{At: now, Key: key, Found: err == nil, Value: v}, name)
	return v, err
}

// fresh reports whether a value stored at t is younger than ttl. A time in the
// future (a clock that moved back) is not fresh.
func fresh(t, now time.Time, ttl time.Duration) bool {
	age := now.Sub(t)
	return age >= 0 && age < ttl
}
