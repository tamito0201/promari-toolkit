package platformtest_test

import (
	"errors"
	"io/fs"
	"slices"
	"testing"
	"time"

	"promari-statusline/internal/infrastructure/platform"
	"promari-statusline/internal/infrastructure/platform/platformtest"
)

var t0 = time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC)

// The fake stands in for the machine in every adapter test, so what it does
// is pinned here: a test that passes against it must mean what it says.
func TestFake(t *testing.T) {
	t.Parallel()

	t.Run("commands and URLs answer what was registered, and are recorded", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		sys.Cmds["git status"] = platformtest.Result{Out: "clean"}
		sys.URLs["https://example.com/x"] = []byte("body")
		if out, err := sys.Run(t.Context(), platform.Cmd{Name: "git", Args: []string{"status"}, Stdin: []byte("in")}); out != "clean" || err != nil {
			t.Errorf("Run = %q, %v", out, err)
		}
		if _, err := sys.Run(t.Context(), platform.Cmd{Name: "gh"}); err == nil {
			t.Error("a command that was not registered ran")
		}
		if body, err := sys.Get(t.Context(), "https://example.com/x", time.Second); string(body) != "body" || err != nil {
			t.Errorf("Get = %q, %v", body, err)
		}
		if _, err := sys.Get(t.Context(), "https://example.com/y", time.Second); err == nil {
			t.Error("a URL that was not registered answered")
		}
		want := []string{"git status", "gh", "GET https://example.com/x", "GET https://example.com/y"}
		if got := sys.Calls(); !slices.Equal(got, want) {
			t.Errorf("Calls() = %q, want %q", got, want)
		}
		if got := string(sys.Stdin("git status")); got != "in" {
			t.Errorf("Stdin() = %q", got)
		}
	})

	t.Run("files", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		if _, err := sys.ReadFile("/a/x.json"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ReadFile of a missing file: %v", err)
		}
		if _, err := sys.ModTime("/a/x.json"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ModTime of a missing file: %v", err)
		}
		if err := sys.WriteFile("/a/x.json", []byte("x"), platform.Private); err != nil {
			t.Fatal(err)
		}
		sys.Files["/a/y.json"] = []byte("y") // put there by a test, without a time
		if got, _ := sys.ModTime("/a/x.json"); !got.Equal(t0) {
			t.Errorf("ModTime of a written file = %v", got)
		}
		if got, _ := sys.ModTime("/a/y.json"); !got.Equal(t0) {
			t.Errorf("ModTime of a file without a time = %v", got)
		}
		if got := sys.Glob("/a/*.json"); !slices.Equal(got, []string{"/a/x.json", "/a/y.json"}) {
			t.Errorf("Glob = %v", got)
		}
		if got := sys.Glob("/a/[bad"); got != nil {
			t.Errorf("Glob of a bad pattern = %v", got)
		}
		data, _ := sys.ReadFile("/a/x.json")
		data[0] = '!'
		if got, _ := sys.File("/a/x.json"); got != "x" {
			t.Errorf("a caller changed the stored file through the slice it read: %q", got)
		}
		if err := sys.Remove("/a/x.json"); err != nil {
			t.Fatal(err)
		}
		if _, ok := sys.File("/a/x.json"); ok {
			t.Error("the file is still there after Remove")
		}
		sys.ReadOnly = true
		if err := sys.WriteFile("/a/z", nil, platform.Private); !errors.Is(err, fs.ErrPermission) {
			t.Errorf("WriteFile on a read-only machine: %v", err)
		}
		if err := sys.Remove("/a/y.json"); !errors.Is(err, fs.ErrPermission) {
			t.Errorf("Remove on a read-only machine: %v", err)
		}
	})

	t.Run("the rest of the machine", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		sys.Env["K"] = "v"
		sys.Path["gh"] = "/bin/gh"
		if sys.Getenv("K") != "v" || sys.HomeDir() != "/h" || sys.Pid() != 100 || sys.NumCPU() != 1 || !sys.Now().Equal(t0) {
			t.Errorf("env, home, pid, cpus, now = %q, %q, %d, %d, %v", sys.Getenv("K"), sys.HomeDir(), sys.Pid(), sys.NumCPU(), sys.Now())
		}
		if path, ok := sys.LookPath("gh"); path != "/bin/gh" || !ok {
			t.Errorf("LookPath = %q, %v", path, ok)
		}
		if _, err := sys.Executable(); err == nil {
			t.Error("Executable() without a running binary succeeded")
		}
		sys.Self = "/bin/psl"
		if path, err := sys.Executable(); path != "/bin/psl" || err != nil {
			t.Errorf("Executable() = %q, %v", path, err)
		}
		if _, ok := sys.TermWidth(); ok {
			t.Error("TermWidth() without a terminal is ok")
		}
		sys.Width = 96
		if width, ok := sys.TermWidth(); width != 96 || !ok {
			t.Errorf("TermWidth() = %d, %v", width, ok)
		}
	})
}
