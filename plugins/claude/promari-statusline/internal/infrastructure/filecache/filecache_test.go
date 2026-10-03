package filecache_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/filecache"
	"promari-statusline/internal/infrastructure/platform"
	"promari-statusline/internal/infrastructure/platform/platformtest"
)

var t0 = time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC)

var errBroken = errors.New("broken")

func TestStorePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		xdg  string
		want string
	}{
		{"under the home directory", "", "/h/.cache/promari-statusline/sessions/s1.json"},
		{"under XDG_CACHE_HOME", "/xdg", "/xdg/promari-statusline/sessions/s1.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Env["XDG_CACHE_HOME"] = tt.xdg
			if got := filecache.NewStore(sys).Path("sessions", "s1.json"); got != tt.want {
				t.Errorf("Path() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadAndSave(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	store := filecache.NewStore(sys)

	if _, ok := filecache.Load[model.Track](store, "track.json"); ok {
		t.Error("Load of a missing file is ok")
	}
	if store.Exists("track.json") {
		t.Error("a missing file exists")
	}
	if err := filecache.Save(store, model.Track{Title: "Take Five"}, "track.json"); err != nil {
		t.Fatal(err)
	}
	if got, ok := filecache.Load[model.Track](store, "track.json"); !ok || got.Title != "Take Five" {
		t.Errorf("Load = %+v, %v", got, ok)
	}
	if !store.Exists("track.json") {
		t.Error("the saved file does not exist")
	}
	if mode := sys.Modes[store.Path("track.json")]; mode != platform.Private {
		t.Errorf("saved with mode %v, want private", mode)
	}

	sys.Files[store.Path("broken.json")] = []byte(`{"title": 5`)
	if got, ok := filecache.Load[model.Track](store, "broken.json"); ok || got != (model.Track{}) {
		t.Errorf("Load of invalid JSON = %+v, %v; want the zero value", got, ok)
	}
	sys.Files[store.Path("wrong.json")] = []byte(`{"title": 5}`)
	if got, ok := filecache.Load[model.Track](store, "wrong.json"); ok || got != (model.Track{}) {
		t.Errorf("Load of another type = %+v, %v; want the zero value", got, ok)
	}

	if err := store.WriteRaw("raw.txt", []byte("as is")); err != nil {
		t.Fatal(err)
	}
	if got, _ := sys.File(store.Path("raw.txt")); got != "as is" {
		t.Errorf("WriteRaw stored %q", got)
	}
	if err := filecache.Save(store, func() {}, "func.json"); err == nil {
		t.Error("Save of a value that has no JSON form succeeded")
	}
}

func TestMemo(t *testing.T) {
	t.Parallel()
	const ttl = time.Minute
	type call struct {
		after time.Duration
		key   string
		value string
		err   error
	}
	tests := []struct {
		name      string
		calls     []call
		wantAsked int
		wantLast  string
		wantErr   error
	}{
		{"asked once, then remembered", []call{{0, "k", "a", nil}, {59 * time.Second, "k", "b", nil}}, 1, "a", nil},
		{"asked again when the answer is as old as the limit", []call{{0, "k", "a", nil}, {ttl, "k", "b", nil}}, 2, "b", nil},
		{"asked again for another key", []call{{0, "k", "a", nil}, {time.Second, "other", "b", nil}}, 2, "b", nil},
		{"nothing to report is remembered too", []call{{0, "k", "", repository.ErrNone}, {time.Second, "k", "b", nil}}, 1, "", repository.ErrNone},
		{"a failure is remembered as nothing", []call{{0, "k", "", errBroken}, {time.Second, "k", "b", nil}}, 1, "", repository.ErrNone},
		{"the failure itself is returned to the first caller", []call{{0, "k", "", errBroken}}, 1, "", errBroken},
		{"an answer from the future is not trusted", []call{{0, "k", "a", nil}, {-time.Hour, "k", "b", nil}}, 2, "b", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			store := filecache.NewStore(sys)
			asked := 0
			var got string
			var err error
			for _, c := range tt.calls {
				sys.T = sys.T.Add(c.after)
				got, err = filecache.Memo(store, "memo.json", c.key, ttl, func() (string, error) {
					asked++
					return c.value, c.err
				})
			}
			if asked != tt.wantAsked || got != tt.wantLast || !errors.Is(err, tt.wantErr) {
				t.Errorf("asked %d times, last = %q, %v; want %d times, %q, %v", asked, got, err, tt.wantAsked, tt.wantLast, tt.wantErr)
			}
		})
	}
	t.Run("a cache that cannot be written asks every time and still answers", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		sys.ReadOnly = true
		store := filecache.NewStore(sys)
		asked := 0
		for range 2 {
			got, err := filecache.Memo(store, "memo.json", "k", ttl, func() (string, error) { asked++; return "a", nil })
			if got != "a" || err != nil {
				t.Fatalf("Memo = %q, %v", got, err)
			}
		}
		if asked != 2 {
			t.Errorf("asked %d times, want 2", asked)
		}
	})
}

// next counts the questions that reach the wrapped reader.
type next struct{ asked int }

func (n *next) PullRequest(context.Context, string, string) (model.PullRequest, error) {
	n.asked++
	return model.PullRequest{Number: n.asked}, nil
}

func (n *next) Track(context.Context) (model.Track, error) {
	n.asked++
	return model.Track{Title: "t"}, nil
}

// Transcript continues from since, as claude.Transcript does: each read adds a
// request to what the earlier reads counted.
func (n *next) Transcript(_ context.Context, _ string, since model.Transcript) (model.Transcript, error) {
	n.asked++
	since.Requests++
	since.Cursor.Offset += 10
	return since, nil
}

func (n *next) Incident(context.Context) (model.Incident, error) {
	n.asked++
	return model.Incident{Indicator: "minor"}, nil
}

func (n *next) Latest(context.Context) (string, error) { n.asked++; return "1.0.0", nil }

func (n *next) Codex(context.Context) (model.CodexLimits, error) {
	n.asked++
	return model.CodexLimits{Balance: model.Some(1.0)}, nil
}

func (n *next) Account(context.Context) (string, error) { n.asked++; return "someone@example.com", nil }

func (n *next) File() string { return "/h/.claude.json" }

func TestDecoratorsRememberTheirAnswers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tests := []struct {
		name string
		ttl  time.Duration
		ask  func(*filecache.Store, *next) error
	}{
		{"pull request", 5 * time.Minute, func(s *filecache.Store, n *next) error {
			_, err := filecache.PullRequests{Store: s, Next: n}.PullRequest(ctx, "/work", "develop")
			return err
		}},
		{"track", 15 * time.Second, func(s *filecache.Store, n *next) error {
			_, err := filecache.Tracks{Store: s, Next: n}.Track(ctx)
			return err
		}},
		{"transcript", 10 * time.Second, func(s *filecache.Store, n *next) error {
			_, err := filecache.Transcripts{Store: s, Next: n}.Transcript(ctx, "/t.jsonl", model.Transcript{})
			return err
		}},
		{"incident", 5 * time.Minute, func(s *filecache.Store, n *next) error {
			_, err := filecache.Incidents{Store: s, Next: n}.Incident(ctx)
			return err
		}},
		{"release", 6 * time.Hour, func(s *filecache.Store, n *next) error {
			_, err := filecache.Releases{Store: s, Next: n}.Latest(ctx)
			return err
		}},
		{"codex", time.Minute, func(s *filecache.Store, n *next) error {
			_, err := filecache.Codex{Store: s, Next: n}.Codex(ctx)
			return err
		}},
		{"account", time.Hour, func(s *filecache.Store, n *next) error {
			_, err := filecache.Accounts{Store: s, Next: n}.Account(ctx)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			store, n := filecache.NewStore(sys), &next{}
			for _, step := range []struct {
				after time.Duration
				want  int
			}{{0, 1}, {tt.ttl - time.Second, 1}, {time.Second, 2}} {
				sys.T = sys.T.Add(step.after)
				if err := tt.ask(store, n); err != nil {
					t.Fatal(err)
				}
				if n.asked != step.want {
					t.Fatalf("after %v: asked %d times, want %d", sys.T.Sub(t0), n.asked, step.want)
				}
			}
		})
	}
	t.Run("the pull request is remembered per directory and branch", func(t *testing.T) {
		t.Parallel()
		store, n := filecache.NewStore(platformtest.New(t0)), &next{}
		pulls := filecache.PullRequests{Store: store, Next: n}
		for _, branch := range []string{"develop", "develop", "feature"} {
			if _, err := pulls.PullRequest(ctx, "/work", branch); err != nil {
				t.Fatal(err)
			}
		}
		if n.asked != 2 {
			t.Errorf("asked %d times for two branches, want 2", n.asked)
		}
	})
}

// login answers the account written in its file, as claude.Account does.
type login struct {
	sys   *platformtest.Fake
	file  string
	asked int
}

func (l *login) File() string { return l.file }

func (l *login) Account(context.Context) (string, error) {
	l.asked++
	data, _ := l.sys.ReadFile(l.file)
	return string(data), nil
}

func TestAccountsFollowTheLoginFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sys := platformtest.New(t0)
	store := filecache.NewStore(sys)
	home := &login{sys: sys, file: "/h/.claude.json"}
	if err := sys.WriteFile(home.file, []byte("a@example.com"), 0o600); err != nil {
		t.Fatal(err)
	}
	ask := func(l *login) string {
		got, err := filecache.Accounts{Store: store, Next: l}.Account(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := ask(home); got != "a@example.com" || home.asked != 1 {
		t.Fatalf("first = %q after %d reads", got, home.asked)
	}
	if got := ask(home); got != "a@example.com" || home.asked != 1 {
		t.Errorf("an unchanged file was read again: %q after %d reads", got, home.asked)
	}

	// /login rewrites the file: the next render sees the new account.
	sys.T = t0.Add(time.Second)
	if err := sys.WriteFile(home.file, []byte("b@example.com"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ask(home); got != "b@example.com" {
		t.Errorf("after /login = %q", got)
	}

	// Another configuration directory is never answered with this one's account.
	work := &login{sys: sys, file: "/work/.claude.json"}
	if err := sys.WriteFile(work.file, []byte("w@example.com"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ask(work); got != "w@example.com" {
		t.Errorf("another directory = %q", got)
	}
}

// reader answers what it is told, and records where each read continued from.
type reader struct {
	answer model.Transcript
	err    error
	since  []int64
}

func (r *reader) Transcript(_ context.Context, _ string, since model.Transcript) (model.Transcript, error) {
	r.since = append(r.since, since.Cursor.Offset)
	return r.answer, r.err
}

func TestTranscriptsContinueWhereTheyStopped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	at := func(offset int64, requests int) model.Transcript {
		return model.Transcript{Requests: requests, Cursor: model.TranscriptCursor{Offset: offset}}
	}
	t.Run("each read continues from the last, per transcript", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		store, n := filecache.NewStore(sys), &next{}
		transcripts := filecache.Transcripts{Store: store, Next: n}
		read := func(path string) model.Transcript {
			sys.T = sys.T.Add(time.Minute)
			got, err := transcripts.Transcript(ctx, path, model.Transcript{})
			if err != nil {
				t.Fatal(err)
			}
			return got
		}
		read("/a.jsonl")
		read("/b.jsonl")
		if got := read("/a.jsonl"); got.Requests != 2 || got.Cursor.Offset != 20 {
			t.Errorf("third read of /a = %+v; a session beside it must not reset its progress", got)
		}
		if got := len(sys.Glob(store.Path("transcripts", "*.json"))); got != 2 {
			t.Errorf("%d files for two transcripts", got)
		}
	})
	t.Run("a failed read keeps the progress", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		store := filecache.NewStore(sys)
		ok := &reader{answer: at(100, 3)}
		if _, err := (filecache.Transcripts{Store: store, Next: ok}).Transcript(ctx, "/t.jsonl", model.Transcript{}); err != nil {
			t.Fatal(err)
		}
		sys.T = t0.Add(time.Minute)
		failing := &reader{err: errors.New("denied")}
		if _, err := (filecache.Transcripts{Store: store, Next: failing}).Transcript(ctx, "/t.jsonl", model.Transcript{}); err == nil {
			t.Fatal("the failure was swallowed")
		}
		sys.T = t0.Add(2 * time.Minute)
		again := &reader{answer: at(150, 4)}
		if _, err := (filecache.Transcripts{Store: store, Next: again}).Transcript(ctx, "/t.jsonl", model.Transcript{}); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(again.since, []int64{100}) {
			t.Errorf("continued from %v, want the offset before the failure", again.since)
		}
	})
	t.Run("nothing to report is remembered with the progress", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		store := filecache.NewStore(sys)
		empty := &reader{answer: at(40, 0), err: repository.ErrNone}
		transcripts := filecache.Transcripts{Store: store, Next: empty}
		for range 2 {
			if got, err := transcripts.Transcript(ctx, "/t.jsonl", model.Transcript{}); !errors.Is(err, repository.ErrNone) || got.Cursor.Offset != 40 {
				t.Fatalf("Transcript() = %+v, %v", got, err)
			}
		}
		if len(empty.since) != 1 {
			t.Errorf("read %d times within the lifetime", len(empty.since))
		}
		sys.T = t0.Add(time.Minute)
		if _, err := transcripts.Transcript(ctx, "/t.jsonl", model.Transcript{}); !errors.Is(err, repository.ErrNone) {
			t.Fatal(err)
		}
		if !slices.Equal(empty.since, []int64{0, 40}) {
			t.Errorf("continued from %v", empty.since)
		}
	})
	t.Run("the records of transcripts no longer read are removed", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		store := filecache.NewStore(sys)
		old := store.Path("transcripts", "0000000000000000.json")
		if err := sys.WriteFile(old, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		sys.T = t0.Add(8 * 24 * time.Hour)
		if _, err := (filecache.Transcripts{Store: store, Next: &reader{answer: at(1, 1)}}).Transcript(ctx, "/new.jsonl", model.Transcript{}); err != nil {
			t.Fatal(err)
		}
		if _, ok := sys.File(old); ok {
			t.Error("the record of a transcript unread for eight days was kept")
		}
	})
}
