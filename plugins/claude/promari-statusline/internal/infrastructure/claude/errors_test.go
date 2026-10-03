package claude_test

import (
	"context"
	"errors"
	"testing"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/infrastructure/claude"
	"promari-statusline/internal/infrastructure/platform/platformtest"
)

var errDenied = errors.New("denied")

// TestReadFailures covers the reads that fail for another reason than a
// missing file: each is reported, not taken for an absent file.
func TestReadFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("the installed binary", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		b := claude.Binary{Sys: sys}
		sys.ReadErr = map[string]error{b.Path(): errDenied}
		if _, err := b.InSync(); !errors.Is(err, errDenied) {
			t.Errorf("InSync() = %v", err)
		}
	})

	t.Run("the to-do list", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		path := claude.ConfigDir(sys) + "/todos/s1-agent-s1.json"
		sys.Files[path] = []byte(`[]`)
		sys.ReadErr = map[string]error{path: errDenied}
		if _, err := (claude.Todos{Sys: sys}).Todos(ctx, "s1"); !errors.Is(err, errDenied) {
			t.Errorf("Todos() = %v", err)
		}
	})

	t.Run("a shorter transcript read again from the start", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		sys.Files["/t.jsonl"] = []byte("{}\n")
		sys.ReadFromErr = func(_ string, offset int64) error {
			if offset == 0 {
				return errDenied
			}
			return nil
		}
		since := model.Transcript{Cursor: model.TranscriptCursor{Offset: 1000, Format: model.TranscriptFormat}}
		if _, err := (claude.Transcript{Sys: sys}).Transcript(ctx, "/t.jsonl", since); !errors.Is(err, errDenied) {
			t.Errorf("Transcript() = %v", err)
		}
	})

	t.Run("the settings", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		s := claude.Settings{Sys: sys}
		sys.ReadErr = map[string]error{s.Path(): errDenied}
		if _, err := s.RemoveStatusLine(); !errors.Is(err, errDenied) {
			t.Errorf("RemoveStatusLine() = %v", err)
		}
	})
}

func TestSettingsThatCannotBeWritten(t *testing.T) {
	t.Parallel()
	t.Run("a value that is not JSON after a name", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		s := claude.Settings{Sys: sys}
		sys.Files[s.Path()] = []byte(`{"model": }`)
		if _, err := s.RemoveStatusLine(); err == nil {
			t.Error("a broken file is left untouched")
		}
		if got, _ := sys.File(s.Path()); got != `{"model": }` {
			t.Errorf("the file became %q", got)
		}
	})
	t.Run("a command that is not UTF-8", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		s := claude.Settings{Sys: sys}
		if _, err := s.SetStatusLine(model.StatusLineSetting{Type: "command", Command: "\xff"}); err == nil {
			t.Error("a command that cannot be encoded is not written")
		}
	})
}
