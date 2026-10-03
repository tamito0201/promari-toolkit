package fsutil_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"promari-model-router/internal/infrastructure/fsutil"
)

func TestExpandHome(t *testing.T) {
	tests := []struct {
		name, home, in, want string
	}{
		{name: "leading ~/ becomes HOME", home: "/h", in: "~/a/b", want: "/h/a/b"},
		{name: "absolute path is unchanged", home: "/h", in: "/x/y", want: "/x/y"},
		{name: "~user is not expanded", home: "/h", in: "~other/x", want: "~other/x"},
		{name: "unknown HOME leaves the path as is", home: "", in: "~/a", want: "~/a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", tt.home)
			if diff := cmp.Diff(tt.want, fsutil.ExpandHome(tt.in)); diff != "" {
				t.Errorf("ExpandHome() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// failingFile wraps a real temporary file and fails one step.
type failingFile struct {
	fsutil.TempFile
	failWrite, failSync, failClose bool
}

var errDisk = errors.New("disk failure")

func (f failingFile) Write(p []byte) (int, error) {
	if f.failWrite {
		return 0, errDisk
	}
	return f.TempFile.Write(p)
}

func (f failingFile) Sync() error {
	if f.failSync {
		return errDisk
	}
	return f.TempFile.Sync()
}

func (f failingFile) Close() error {
	err := f.TempFile.Close()
	if f.failClose {
		return errDisk
	}
	return err
}

func TestWriteFileAtomic(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, dir string) string // returns the target path
		wrap    func(fsutil.TempFile) fsutil.TempFile // nil = the real file
		failNew bool
		wantErr bool
	}{
		{name: "writes 0600 and replaces an existing file", prepare: func(t *testing.T, dir string) string {
			t.Helper()
			path := filepath.Join(dir, "a.json")
			if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			return path
		}},
		{name: "the temporary file cannot be created", failNew: true, wantErr: true},
		{name: "a failed write leaves nothing behind", wrap: func(f fsutil.TempFile) fsutil.TempFile { return failingFile{TempFile: f, failWrite: true} }, wantErr: true},
		{name: "a failed sync leaves nothing behind", wrap: func(f fsutil.TempFile) fsutil.TempFile { return failingFile{TempFile: f, failSync: true} }, wantErr: true},
		{name: "a failed close leaves nothing behind", wrap: func(f fsutil.TempFile) fsutil.TempFile { return failingFile{TempFile: f, failClose: true} }, wantErr: true},
		{name: "a failed rename leaves nothing behind", wantErr: true, prepare: func(t *testing.T, dir string) string {
			t.Helper()
			path := filepath.Join(dir, "a.json")
			if err := os.MkdirAll(filepath.Join(path, "occupied"), 0o700); err != nil {
				t.Fatal(err)
			}
			return path
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "a.json")
			if tt.prepare != nil {
				path = tt.prepare(t, dir)
			}
			if tt.wrap != nil || tt.failNew {
				t.Cleanup(fsutil.SetCreateTemp(func(d, pattern string) (fsutil.TempFile, error) {
					if tt.failNew {
						return nil, errDisk
					}
					f, err := os.CreateTemp(d, pattern)
					if err != nil {
						return nil, err
					}
					return tt.wrap(f), nil
				}))
			}
			err := fsutil.WriteFileAtomic(path, []byte("new"))
			if (err != nil) != tt.wantErr {
				t.Fatalf("WriteFileAtomic() = %v, wantErr %v", err, tt.wantErr)
			}
			entries, _ := os.ReadDir(dir)
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			wantNames := []string{"a.json"}
			if tt.failNew || tt.wrap != nil {
				wantNames = nil
			}
			if diff := cmp.Diff(wantNames, names, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("files left in the directory (-want +got):\n%s", diff)
			}
			if tt.wantErr {
				return
			}
			raw, err := os.ReadFile(path)
			info, statErr := os.Stat(path)
			if err != nil || statErr != nil {
				t.Fatal(err, statErr)
			}
			if diff := cmp.Diff([]any{"new", os.FileMode(0o600)}, []any{string(raw), info.Mode().Perm()}); diff != "" {
				t.Errorf("content and mode (-want +got):\n%s", diff)
			}
		})
	}
}
