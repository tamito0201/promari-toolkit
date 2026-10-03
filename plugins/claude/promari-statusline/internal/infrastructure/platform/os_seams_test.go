package platform

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The tests of this file swap the package's calls to the operating system and
// so do not run in parallel: Go runs the parallel tests after them.

var errSeam = errors.New("the operating system refused")

// brokenFile is a temporary file whose write or chmod fails.
type brokenFile struct {
	name                 string
	failWrite, failChmod bool
}

func (b brokenFile) Write(p []byte) (int, error) {
	if b.failWrite {
		return 0, errSeam
	}
	return len(p), nil
}

func (b brokenFile) Chmod(fs.FileMode) error {
	if b.failChmod {
		return errSeam
	}
	return nil
}

func (brokenFile) Close() error   { return nil }
func (b brokenFile) Name() string { return b.name }

func swap[T any](t *testing.T, v *T, with T) {
	t.Helper()
	old := *v
	*v = with
	t.Cleanup(func() { *v = old })
}

func TestWriteFileFailures(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []brokenFile{{failWrite: true}, {failChmod: true}} {
		f.name = filepath.Join(dir, "x.tmp")
		swap(t, &createTemp, func(string, string) (tempFile, error) { return f, nil })
		err := New().WriteFile(filepath.Join(dir, "x"), []byte("x"), 0o600)
		if !errors.Is(err, errSeam) {
			t.Errorf("WriteFile() with %+v = %v", f, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "x")); !errors.Is(err, fs.ErrNotExist) {
			t.Error("a failed write leaves the target alone")
		}
	}
}

func TestReadFromStatFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	swap(t, &statFile, func(*os.File) (fs.FileInfo, error) { return nil, errSeam })
	if _, _, err := New().ReadFrom(path, 0); !errors.Is(err, errSeam) {
		t.Errorf("ReadFrom() = %v", err)
	}
}

func TestExecutableFailure(t *testing.T) {
	swap(t, &executable, func() (string, error) { return "", errSeam })
	if _, err := New().Executable(); !errors.Is(err, errSeam) {
		t.Errorf("Executable() = %v", err)
	}
}

func TestTermWidthOfATerminal(t *testing.T) {
	tty := filepath.Join(t.TempDir(), "tty")
	if err := os.WriteFile(tty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	swap(t, &getSize, func(int) (int, int, error) { return 132, 40, nil })
	o := New()
	o.tty = tty
	if width, ok := o.TermWidth(); !ok || width != 132 {
		t.Errorf("TermWidth() = %d, %v", width, ok)
	}
}

func TestGetBodyCutShort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Promise more than is sent: the body ends early.
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	defer srv.Close()
	if _, err := New().Get(context.Background(), srv.URL, time.Second); err == nil {
		t.Error("a body cut short is an error")
	}
}
