package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"golang.org/x/term"
)

const (
	// waitDelay is how long a killed child may keep its pipes open before Run
	// gives up on them (a grandchild can inherit the pipe and outlive the kill).
	waitDelay = 200 * time.Millisecond
	// maxBody bounds an HTTP response; the endpoints read are a few kilobytes.
	maxBody = 1 << 20
	// dirMode keeps the cache and its session data private to the user.
	dirMode = 0o700
	// controllingTerminal is the device of the terminal the process belongs to.
	controllingTerminal = "/dev/tty"
)

// OS is the System of the real machine.
type OS struct {
	client *http.Client
	tty    string
}

// New returns the System of the real machine.
func New() *OS {
	return &OS{client: &http.Client{}, tty: controllingTerminal}
}

// Now returns the current time.
func (*OS) Now() time.Time { return time.Now() }

// Getenv returns an environment variable.
func (*OS) Getenv(key string) string { return os.Getenv(key) }

// HomeDir returns the user's home directory.
func (*OS) HomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// Pid returns this process's id.
func (*OS) Pid() int { return os.Getpid() }

// Ppid returns the id of the parent process.
func (*OS) Ppid() int { return os.Getppid() }

// NumCPU returns the number of logical CPUs.
func (*OS) NumCPU() int { return runtime.NumCPU() }

// Run runs a child process. The timeout ends the wait and kills the process;
// WaitDelay then ends the wait for its pipes, so Run always returns.
func (*OS) Run(ctx context.Context, c Cmd) (string, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.WaitDelay = waitDelay
	if c.Stdin != nil {
		cmd.Stdin = bytes.NewReader(c.Stdin)
	}
	out, err := cmd.Output()
	if err != nil {
		return string(out), fmt.Errorf("run %s: %w", c.Name, err)
	}
	return string(out), nil
}

// Get fetches a URL. The timeout ends this side's wait; the servers read are
// public status endpoints, so nothing has to be stopped on their side.
func (o *OS) Get(ctx context.Context, url string, timeout time.Duration) (body []byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s: status %d", url, resp.StatusCode)
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	return body, nil
}

// ReadFile returns a file's content.
func (*OS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// ReadFrom returns a file's content from offset to its end, and its size.
func (*OS) ReadFrom(path string, offset int64) (data []byte, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", path, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", path, err)
	}
	size = info.Size()
	if offset < 0 || offset >= size {
		return nil, size, nil
	}
	data = make([]byte, size-offset)
	n, err := f.ReadAt(data, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, 0, fmt.Errorf("read %s: %w", path, err)
	}
	return data[:n], size, nil
}

// WriteFile writes to a temporary file beside the target and renames it over
// the target, so another session's status line never reads a half-written file.
func (*OS) WriteFile(path string, data []byte, mode fs.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(tmp.Name()))
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return errors.Join(fmt.Errorf("write %s: %w", path, err), tmp.Close())
	}
	// Chmod after the write: the temporary file is created private, so its
	// content is never readable by others under a wider final mode.
	if err := errors.Join(tmp.Chmod(mode), tmp.Close()); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Remove deletes a file; a file that does not exist is not an error.
func (*OS) Remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// LookPath finds an executable on PATH.
func (*OS) LookPath(name string) (string, bool) {
	path, err := exec.LookPath(name)
	return path, err == nil
}

// Executable returns the path of the running binary, with symbolic links
// resolved.
func (*OS) Executable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find the running binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return path, nil
}

// ModTime returns when a file was last modified.
func (*OS) ModTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("stat %s: %w", path, err)
	}
	return info.ModTime(), nil
}

// Glob returns the paths matching a pattern, sorted.
func (*OS) Glob(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	slices.Sort(matches)
	return matches
}

// TermWidth asks the controlling terminal for its width. A status line often
// has no controlling terminal; ok is then false.
func (o *OS) TermWidth() (width int, ok bool) {
	tty, err := os.Open(o.tty)
	if err != nil {
		return 0, false
	}
	width, _, sizeErr := term.GetSize(int(tty.Fd()))
	if err := errors.Join(sizeErr, tty.Close()); err != nil {
		return 0, false
	}
	return width, true
}
