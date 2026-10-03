// Package fsutil holds the file-system helpers every adapter shares, so that
// the adapters do not depend on one another for them.
package fsutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome replaces a leading "~/" with the home directory.
func ExpandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return path
}

// tempFile is the part of *os.File WriteFileAtomic uses (an interface so that
// tests can make Write, Sync and Close fail, which a local disk rarely does).
type tempFile interface {
	Name() string
	Write(p []byte) (int, error)
	Sync() error
	Close() error
}

// createTemp creates the temporary file (a variable for the tests).
var createTemp = func(dir, pattern string) (tempFile, error) { return os.CreateTemp(dir, pattern) }

// WriteFileAtomic writes data to path through a temporary file in the same
// directory (mode 0600), flushed to disk and renamed over path, so a reader
// sees the old file or the new one, never half of one. The temporary file is
// removed when any step fails.
func WriteFileAtomic(path string, data []byte) (err error) {
	f, err := createTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(f.Name())
		}
	}()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
