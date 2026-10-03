package platform

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestOSBasics(t *testing.T) {
	t.Setenv("PSL_TEST_VARIABLE", "set")
	sys := New()
	if got := sys.Getenv("PSL_TEST_VARIABLE"); got != "set" {
		t.Errorf("Getenv = %q", got)
	}
	if time.Since(sys.Now()) > time.Minute {
		t.Errorf("Now() = %v", sys.Now())
	}
	if sys.Pid() != os.Getpid() || sys.NumCPU() < 1 {
		t.Errorf("Pid, NumCPU = %d, %d", sys.Pid(), sys.NumCPU())
	}
	t.Setenv("HOME", "/somewhere")
	if got := sys.HomeDir(); got != "/somewhere" {
		t.Errorf("HomeDir() = %q", got)
	}
	t.Setenv("HOME", "")
	if got := sys.HomeDir(); got != "" {
		t.Errorf("HomeDir() without HOME = %q", got)
	}
	if path, ok := sys.LookPath("sh"); !ok || !filepath.IsAbs(path) {
		t.Errorf("LookPath(sh) = %q, %v", path, ok)
	}
	if _, ok := sys.LookPath("no-such-tool-psl"); ok {
		t.Error("LookPath found a tool that does not exist")
	}
	if path, err := sys.Executable(); err != nil || !filepath.IsAbs(path) {
		t.Errorf("Executable() = %q, %v", path, err)
	}
}

func TestOSRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tests := []struct {
		name    string
		cmd     Cmd
		want    string
		wantErr bool
	}{
		{"the output", Cmd{Name: "sh", Args: []string{"-c", "echo out; echo err >&2"}}, "out\n", false},
		{"standard input", Cmd{Name: "cat", Stdin: []byte("in")}, "in", false},
		{"the working directory", Cmd{Name: "pwd", Dir: dir}, "", false},
		{"a failing command still gives its output", Cmd{Name: "sh", Args: []string{"-c", "echo partial; exit 3"}}, "partial\n", true},
		{"a command that does not exist", Cmd{Name: "no-such-tool-psl"}, "", true},
		{"a command slower than its timeout is killed", Cmd{Name: "sleep", Args: []string{"30"}, Timeout: 50 * time.Millisecond}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start := time.Now()
			got, err := New().Run(t.Context(), tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want an error: %v", err, tt.wantErr)
			}
			if tt.cmd.Dir != "" {
				// macOS reaches the temporary directory through a symbolic link.
				if resolved, _ := filepath.EvalSymlinks(tt.cmd.Dir); strings.TrimSpace(got) != resolved && strings.TrimSpace(got) != tt.cmd.Dir {
					t.Errorf("ran in %q, want %q", strings.TrimSpace(got), tt.cmd.Dir)
				}
			} else if got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
			if elapsed := time.Since(start); elapsed > 10*time.Second {
				t.Errorf("took %v", elapsed)
			}
		})
	}
	t.Run("a failure names the command and keeps the exit status", func(t *testing.T) {
		t.Parallel()
		_, err := New().Run(t.Context(), Cmd{Name: "sh", Args: []string{"-c", "exit 3"}})
		exit, ok := errors.AsType[*exec.ExitError](err)
		if !ok || exit.ExitCode() != 3 || !strings.Contains(err.Error(), "run sh") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("a cancelled context stops the command", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := New().Run(ctx, Cmd{Name: "sleep", Args: []string{"30"}, Timeout: time.Minute}); err == nil {
			t.Error("a command ran to its end under a cancelled context")
		}
	})
	t.Run("a grandchild that keeps the pipe open does not hold Run", func(t *testing.T) {
		t.Parallel()
		start := time.Now()
		_, err := New().Run(t.Context(), Cmd{Name: "sh", Args: []string{"-c", "sleep 30 & wait"}, Timeout: 50 * time.Millisecond})
		if err == nil || time.Since(start) > 10*time.Second {
			t.Errorf("err = %v after %v", err, time.Since(start))
		}
	})
}

func TestOSGet(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/large":
			_, _ = w.Write([]byte(strings.Repeat("x", maxBody+100)))
		case "/slow":
			select {
			case <-r.Context().Done():
			case <-time.After(30 * time.Second):
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	tests := []struct {
		name    string
		url     string
		timeout time.Duration
		want    int
		wantErr string
	}{
		{"a response", server.URL + "/ok", time.Second, len(`{"ok":true}`), ""},
		{"a response larger than the limit is cut", server.URL + "/large", time.Second, maxBody, ""},
		{"a status other than 200", server.URL + "/missing", time.Second, 0, "status 404"},
		{"a server slower than the timeout", server.URL + "/slow", 50 * time.Millisecond, 0, "deadline exceeded"},
		{"an address that is not a URL", "://nowhere", time.Second, 0, "get ://nowhere"},
		{"a server that is not there", "http://127.0.0.1:1/x", time.Second, 0, "get http://127.0.0.1:1/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body, err := New().Get(t.Context(), tt.url, tt.timeout)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || len(body) != tt.want {
				t.Errorf("Get = %d bytes, %v; want %d", len(body), err, tt.want)
			}
		})
	}
}

func TestOSFiles(t *testing.T) {
	t.Parallel()
	sys := New()
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "file.json")

	if _, err := sys.ReadFile(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile of a missing file: %v", err)
	}
	if _, err := sys.ModTime(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ModTime of a missing file: %v", err)
	}
	if err := sys.Remove(path); err != nil {
		t.Errorf("Remove of a missing file: %v", err)
	}

	for _, content := range []string{"first", "second"} {
		if err := sys.WriteFile(path, []byte(content), Private); err != nil {
			t.Fatal(err)
		}
		if got, err := sys.ReadFile(path); err != nil || string(got) != content {
			t.Errorf("ReadFile = %q, %v; want %q", got, err, content)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != Private {
		t.Errorf("mode = %v, %v; want %v", info.Mode().Perm(), err, Private)
	}
	if parent, err := os.Stat(filepath.Dir(path)); err != nil || parent.Mode().Perm() != dirMode {
		t.Errorf("directory mode = %v, %v; want %v", parent.Mode().Perm(), err, os.FileMode(dirMode))
	}
	if modified, err := sys.ModTime(path); err != nil || time.Since(modified) > time.Minute {
		t.Errorf("ModTime = %v, %v", modified, err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("WriteFile left temporary files behind: %v", entries)
	}

	binary := filepath.Join(dir, "psl")
	if err := sys.WriteFile(binary, []byte("#!/bin/sh\n"), Executable); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(binary); err != nil || info.Mode().Perm() != Executable {
		t.Errorf("mode of an executable = %v, %v", info.Mode().Perm(), err)
	}

	if err := sys.WriteFile(filepath.Join(dir, "x.json"), nil, Private); err != nil {
		t.Fatal(err)
	}
	if got := sys.Glob(filepath.Join(dir, "*")); !slices.Equal(got, []string{filepath.Join(dir, "a"), binary, filepath.Join(dir, "x.json")}) {
		t.Errorf("Glob = %v", got)
	}
	if got := sys.Glob("[bad pattern"); got != nil {
		t.Errorf("Glob of a bad pattern = %v", got)
	}

	if err := sys.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := sys.ReadFile(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the file is still there after Remove: %v", err)
	}
}

func TestOSFileFailures(t *testing.T) {
	t.Parallel()
	sys := New()
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, Private); err != nil {
		t.Fatal(err)
	}
	t.Run("the directory cannot be created", func(t *testing.T) {
		t.Parallel()
		if err := sys.WriteFile(filepath.Join(file, "child", "x"), nil, Private); err == nil {
			t.Error("WriteFile under a file succeeded")
		}
	})
	t.Run("the directory cannot be written", func(t *testing.T) {
		t.Parallel()
		locked := filepath.Join(t.TempDir(), "locked")
		if err := os.Mkdir(locked, 0o500); err != nil {
			t.Fatal(err)
		}
		if err := sys.WriteFile(filepath.Join(locked, "x"), nil, Private); err == nil {
			t.Error("WriteFile in a read-only directory succeeded")
		}
	})
	t.Run("the target is a directory", func(t *testing.T) {
		t.Parallel()
		target := filepath.Join(t.TempDir(), "target")
		if err := os.MkdirAll(filepath.Join(target, "child"), dirMode); err != nil {
			t.Fatal(err)
		}
		if err := sys.WriteFile(target, []byte("x"), Private); err == nil {
			t.Error("WriteFile over a directory with content succeeded")
		}
		if entries, _ := os.ReadDir(filepath.Dir(target)); len(entries) != 1 {
			t.Errorf("a failed WriteFile left its temporary file behind: %v", entries)
		}
	})
	t.Run("a directory with content cannot be removed", func(t *testing.T) {
		t.Parallel()
		if err := sys.Remove(dir); err == nil {
			t.Error("Remove of a directory with content succeeded")
		}
	})
}

func TestOSTermWidth(t *testing.T) {
	t.Parallel()
	notATerminal := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notATerminal, nil, Private); err != nil {
		t.Fatal(err)
	}
	for name, tty := range map[string]string{"no controlling terminal": "/no/such/device", "a file that is not a terminal": notATerminal} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sys := New()
			sys.tty = tty
			if width, ok := sys.TermWidth(); ok {
				t.Errorf("TermWidth() = %d, true", width)
			}
		})
	}
	t.Run("the real terminal, when the test has one", func(t *testing.T) {
		t.Parallel()
		if width, ok := New().TermWidth(); ok && width <= 0 {
			t.Errorf("TermWidth() = %d, true", width)
		}
	})
}

func TestOSProcesses(t *testing.T) {
	sys := New()
	if got := sys.Ppid(); got != os.Getppid() {
		t.Errorf("Ppid = %d, want %d", got, os.Getppid())
	}
	if alive, ok := sys.Alive(os.Getpid()); ok && !alive {
		t.Error("this process is not alive")
	}
	if alive, ok := sys.Alive(0); ok && alive {
		t.Error("process 0 is alive")
	}
	// A child that has exited and been waited for is gone.
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if alive, ok := sys.Alive(cmd.Process.Pid); ok && alive {
		t.Errorf("an exited child %d is alive", cmd.Process.Pid)
	}
	if ppid, name, ok := sys.Parent(os.Getpid()); ok && (ppid != os.Getppid() || name == "") {
		t.Errorf("Parent = %d %q, want %d", ppid, name, os.Getppid())
	}
	if _, _, ok := sys.Parent(1 << 30); ok {
		t.Error("a process that does not exist has a parent")
	}
}
