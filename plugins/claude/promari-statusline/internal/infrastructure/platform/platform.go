// Package platform is the adapters' only door to the machine: the clock, the
// environment, files, child processes, HTTP and the terminal. Every adapter in
// internal/infrastructure reaches them through System, so an adapter can be
// tested with the in-memory fake and no test starts a real process or opens a
// real socket.
package platform

import (
	"context"
	"io/fs"
	"time"
)

// File modes: what the plugin stores is private to the user; the installed
// binary must be executable.
const (
	Private    fs.FileMode = 0o600
	Executable fs.FileMode = 0o755
)

// Cmd describes one child process.
type Cmd struct {
	// Name is looked up on PATH unless it contains a path separator.
	Name string
	Args []string
	// Dir is the working directory; empty means the caller's.
	Dir string
	// Stdin is written to the process; nil leaves it empty.
	Stdin []byte
	// Timeout bounds the whole run; zero means DefaultTimeout.
	Timeout time.Duration
}

// DefaultTimeout bounds a child process that sets no timeout of its own.
const DefaultTimeout = 2 * time.Second

// System is what the status line needs from the machine it runs on.
type System interface {
	// Now returns the current time.
	Now() time.Time
	// Getenv returns an environment variable, or "" when it is unset.
	Getenv(key string) string
	// HomeDir returns the user's home directory, or "" when it is unknown.
	HomeDir() string
	// Pid returns the status line's own process id.
	Pid() int
	// Ppid returns the id of the process that started the status line: the
	// Claude Code of the session (or a shell between them).
	Ppid() int
	// Parent returns a process's parent and command name. ok is false when
	// the system cannot tell.
	Parent(pid int) (ppid int, name string, ok bool)
	// Alive reports whether a process is running. ok is false where the
	// question cannot be asked (Windows); alive is then meaningless.
	Alive(pid int) (alive, ok bool)
	// NumCPU returns the number of logical CPUs.
	NumCPU() int
	// Run runs a child process and returns its standard output. The output is
	// returned even when the process exits with a non-zero status; the error
	// then says so.
	Run(ctx context.Context, c Cmd) (string, error)
	// Get fetches a URL and returns the body of a 200 response.
	Get(ctx context.Context, url string, timeout time.Duration) ([]byte, error)
	// ReadFile returns a file's content.
	ReadFile(path string) ([]byte, error)
	// WriteFile replaces a file atomically, creating its directory.
	WriteFile(path string, data []byte, mode fs.FileMode) error
	// Remove deletes a file; a file that does not exist is not an error.
	Remove(path string) error
	// LookPath finds an executable on PATH.
	LookPath(name string) (string, bool)
	// Executable returns the path of the running binary.
	Executable() (string, error)
	// ModTime returns when a file was last modified.
	ModTime(path string) (time.Time, error)
	// Glob returns the paths matching a pattern, sorted; none on any error.
	Glob(pattern string) []string
	// TermWidth returns the width of the controlling terminal in cells.
	TermWidth() (int, bool)
}
