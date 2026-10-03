// Package filecache keeps small JSON files in the status line's cache
// directory, and remembers the answers of slow sources between renders.
//
// The status line is a new process on every render, several times a second
// while a session is busy. Whatever takes longer than a few milliseconds (a
// GitHub lookup, a status page, a scan of a transcript) is asked once and then
// read from here until it is old.
package filecache

import (
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

// Load reads a file of the store as T. ok is false when the file is missing or
// does not hold a T; the zero value is then returned.
func Load[T any](s *Store, name ...string) (v T, ok bool) {
	data, err := s.sys.ReadFile(s.Path(name...))
	if err != nil {
		return v, false
	}
	if err := json.Unmarshal(data, &v); err != nil {
		var zero T
		return zero, false
	}
	return v, true
}

// Save writes v to a file of the store.
func Save[T any](s *Store, v T, name ...string) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode %s: %w", s.Path(name...), err)
	}
	return s.sys.WriteFile(s.Path(name...), data, platform.Private)
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

// Memo returns the answer remembered for key while it is younger than ttl, and
// otherwise asks and remembers the answer. A remembered "nothing" is returned
// as repository.ErrNone.
func Memo[T any](s *Store, name, key string, ttl time.Duration, ask func() (T, error)) (T, error) {
	now := s.sys.Now()
	if e, ok := Load[entry[T]](s, name); ok && e.Key == key && fresh(e.At, now, ttl) {
		if !e.Found {
			var zero T
			return zero, repository.ErrNone
		}
		return e.Value, nil
	}
	v, err := ask()
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
