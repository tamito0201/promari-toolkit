// Package platformtest provides a System that lives in memory, for tests. Every
// method is safe for concurrent use, because the status line collects its data
// in parallel.
package platformtest

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"promari-statusline/internal/infrastructure/platform"
)

// Result is what a faked command prints and returns.
type Result struct {
	Out string
	Err error
}

// errNoCommand is returned for a command the test did not provide.
var errNoCommand = errors.New("command not found")

// errNoURL is returned for a URL the test did not provide.
var errNoURL = errors.New("no such url")

// Fake is an in-memory System.
type Fake struct {
	mu sync.Mutex

	// T is the current time.
	T time.Time
	// Env holds the environment variables.
	Env map[string]string
	// Home is the home directory.
	Home string
	// ID is the process id.
	ID int
	// PPID is the parent's process id.
	PPID int
	// Procs maps a process id to its parent and command name, for Parent.
	Procs map[int]Proc
	// Running holds the ids of the processes that are alive; nil makes Alive
	// unable to tell, as on Windows.
	Running map[int]bool
	// CPUs is the number of logical CPUs.
	CPUs int
	// Files maps a path to its content.
	Files map[string][]byte
	// Times maps a path to its modification time; files written through
	// WriteFile get T.
	Times map[string]time.Time
	// Cmds maps "name arg1 arg2" to what the command prints.
	Cmds map[string]Result
	// URLs maps a URL to its body.
	URLs map[string][]byte
	// Width is the terminal width; zero means no controlling terminal.
	Width int
	// ReadOnly makes WriteFile and Remove fail.
	ReadOnly bool
	// Path maps the name of an executable on PATH to its location.
	Path map[string]string
	// Self is the path of the running binary; empty makes Executable fail.
	Self string
	// Modes holds the mode each file was written with.
	Modes map[string]fs.FileMode

	calls []string
	stdin map[string][]byte
}

// The process ids a new Fake runs as: the status line and the Claude Code
// that started it.
const (
	fakePid    = 100
	fakeParent = 90
)

// New returns a Fake at the given time with an empty machine.
func New(now time.Time) *Fake {
	return &Fake{
		T:     now,
		Env:   map[string]string{},
		Home:  "/h",
		ID:    fakePid,
		PPID:  fakeParent,
		CPUs:  1,
		Files: map[string][]byte{},
		Times: map[string]time.Time{},
		Cmds:  map[string]Result{},
		URLs:  map[string][]byte{},
		Path:  map[string]string{},
		Modes: map[string]fs.FileMode{},
		stdin: map[string][]byte{},
	}
}

var _ platform.System = (*Fake)(nil)

// Now returns T.
func (f *Fake) Now() time.Time { return f.T }

// Getenv returns Env[key].
func (f *Fake) Getenv(key string) string { return f.Env[key] }

// HomeDir returns Home.
func (f *Fake) HomeDir() string { return f.Home }

// Pid returns ID.
func (f *Fake) Pid() int { return f.ID }

// Ppid returns PPID.
func (f *Fake) Ppid() int { return f.PPID }

// Proc is a faked process: its parent and command name.
type Proc struct {
	PPID int
	Name string
}

// Parent returns the process in Procs.
func (f *Fake) Parent(pid int) (ppid int, name string, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.Procs[pid]
	return p.PPID, p.Name, ok
}

// Alive reports whether pid is in Running; ok is false while Running is nil.
func (f *Fake) Alive(pid int) (alive, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Running == nil {
		return false, false
	}
	return f.Running[pid], true
}

// NumCPU returns CPUs.
func (f *Fake) NumCPU() int { return f.CPUs }

// Run returns the Result registered for the command line.
func (f *Fake) Run(_ context.Context, c platform.Cmd) (string, error) {
	key := strings.Join(append([]string{c.Name}, c.Args...), " ")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, key)
	f.stdin[key] = c.Stdin
	r, ok := f.Cmds[key]
	if !ok {
		return "", errNoCommand
	}
	return r.Out, r.Err
}

// Get returns the body registered for the URL.
func (f *Fake) Get(_ context.Context, url string, _ time.Duration) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "GET "+url)
	body, ok := f.URLs[url]
	if !ok {
		return nil, errNoURL
	}
	return body, nil
}

// ReadFile returns Files[path].
func (f *Fake) ReadFile(name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.Files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return slices.Clone(data), nil
}

// WriteFile stores the content and stamps it with T.
func (f *Fake) WriteFile(name string, data []byte, mode fs.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ReadOnly {
		return fs.ErrPermission
	}
	f.Files[name] = slices.Clone(data)
	f.Times[name] = f.T
	f.Modes[name] = mode
	return nil
}

// Remove deletes a stored file.
func (f *Fake) Remove(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ReadOnly {
		return fs.ErrPermission
	}
	delete(f.Files, name)
	delete(f.Times, name)
	return nil
}

// LookPath returns Path[name].
func (f *Fake) LookPath(name string) (string, bool) {
	p, ok := f.Path[name]
	return p, ok
}

// errNoSelf is returned when the test gave the fake no running binary.
var errNoSelf = errors.New("no running binary")

// Executable returns Self.
func (f *Fake) Executable() (string, error) {
	if f.Self == "" {
		return "", errNoSelf
	}
	return f.Self, nil
}

// ModTime returns Times[path], or T for a file that has no entry.
func (f *Fake) ModTime(name string) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.Files[name]; !ok {
		return time.Time{}, fs.ErrNotExist
	}
	if t, ok := f.Times[name]; ok {
		return t, nil
	}
	return f.T, nil
}

// Glob matches the pattern against the stored paths.
func (f *Fake) Glob(pattern string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for name := range f.Files {
		if ok, err := path.Match(pattern, name); err == nil && ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// TermWidth returns Width.
func (f *Fake) TermWidth() (int, bool) { return f.Width, f.Width > 0 }

// Calls returns the commands and URLs requested so far, in order.
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// Stdin returns what the command line last received on standard input.
func (f *Fake) Stdin(key string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stdin[key]
}

// File returns a stored file as a string, and whether it exists.
func (f *Fake) File(name string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.Files[name]
	return string(data), ok
}
