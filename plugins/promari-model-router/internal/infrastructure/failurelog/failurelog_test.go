package failurelog_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/infrastructure/failurelog"
)

// put writes body at path, creating the directory.
func put(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFile(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 30, 0, 0, time.FixedZone("JST", 9*3600))
	type result struct {
		Failure model.Failure
		OK      bool
		Err     string // substring
	}
	tests := []struct {
		name    string
		prepare func(t *testing.T, path string) // writes the file directly
		record  *model.Failure
		want    result
		wantRec string // substring of the RecordFailure error
	}{
		{name: "no file is no failure"},
		{
			name: "a recorded failure reads back in UTC", record: &model.Failure{At: at, Message: "PreToolUse: boom\nsecond line"},
			want: result{Failure: model.Failure{At: at.UTC(), Message: "PreToolUse: boom\nsecond line"}, OK: true},
		},
		{
			name: "the launcher's format (a message without a trailing newline)",
			prepare: func(t *testing.T, path string) {
				t.Helper()
				put(t, path, "2026-10-01T00:30:00Z\nchecksum mismatch")
			},
			want: result{Failure: model.Failure{At: at.UTC(), Message: "checksum mismatch"}, OK: true},
		},
		{
			name: "a first line that is not a time is an error",
			prepare: func(t *testing.T, path string) {
				t.Helper()
				put(t, path, "yesterday\nboom\n")
			},
			want: result{Err: "first line is not an RFC 3339 time"},
		},
		{
			name: "an unreadable file is an error",
			prepare: func(t *testing.T, path string) {
				t.Helper()
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
			},
			want: result{Err: "is a directory"},
		},
		{
			name: "the directory cannot be created",
			prepare: func(t *testing.T, path string) {
				t.Helper()
				put(t, filepath.Dir(path), "")
			},
			record: &model.Failure{At: at, Message: "x"}, wantRec: "not a directory", want: result{Err: "not a directory"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "data", "last_error")
			if tt.prepare != nil {
				tt.prepare(t, path)
			}
			l := failurelog.File{Path: path}
			if tt.record != nil {
				err := l.RecordFailure(*tt.record)
				if (err == nil) != (tt.wantRec == "") || err != nil && !strings.Contains(err.Error(), tt.wantRec) {
					t.Fatalf("RecordFailure() = %v, want %q", err, tt.wantRec)
				}
			}
			f, ok, err := l.LastFailure()
			got := result{Failure: f, OK: ok}
			if err != nil && strings.Contains(err.Error(), tt.want.Err) {
				got.Err = tt.want.Err
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("LastFailure() (-want +got):\n%s\nerror: %v", diff, err)
			}
		})
	}
}
