package state_test

import (
	"slices"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/infrastructure/filecache"
	"promari-statusline/internal/infrastructure/platform/platformtest"
	"promari-statusline/internal/infrastructure/state"
)

func peers(sys *platformtest.Fake) state.Peers {
	return state.Peers{Store: filecache.NewStore(sys), Sys: sys}
}

func keys(r model.Roster) []string {
	out := make([]string, 0, len(r))
	for i := range r {
		out = append(out, r[i].Key)
	}
	slices.Sort(out)
	return out
}

// post writes a peer as the status line of the given Claude Code would.
func post(t *testing.T, sys *platformtest.Fake, owner int, p model.Peer) {
	t.Helper()
	sys.PPID = owner
	if err := peers(sys).Post(p); err != nil {
		t.Fatal(err)
	}
}

func TestPeersPostStampsTheClaudeCodeThatRunsTheSession(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		procs map[int]platformtest.Proc
		want  int
	}{
		{"the parent is Claude Code", map[int]platformtest.Proc{50: {PPID: 10, Name: "claude"}}, 50},
		{"a shell stands between", map[int]platformtest.Proc{50: {PPID: 40, Name: "/bin/zsh"}, 40: {PPID: 10, Name: "claude"}}, 40},
		{"two shells stand between", map[int]platformtest.Proc{50: {PPID: 45, Name: "sh"}, 45: {PPID: 40, Name: "bash"}, 40: {PPID: 10, Name: "claude"}}, 40},
		{"the system cannot tell", nil, 50},
		{"a shell started by init is kept", map[int]platformtest.Proc{50: {PPID: 1, Name: "sh"}}, 50},
		{"a chain of shells ends the walk", map[int]platformtest.Proc{
			50: {PPID: 49, Name: "sh"}, 49: {PPID: 48, Name: "sh"}, 48: {PPID: 47, Name: "sh"}, 47: {PPID: 46, Name: "sh"}, 46: {PPID: 45, Name: "sh"},
		}, 46},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Procs = c.procs
			post(t, sys, 50, model.Peer{Key: "s1", At: t0})
			got, ok := filecache.Load[model.Peer](filecache.NewStore(sys), "peers", "s1.json")
			if !ok || got.Owner != c.want {
				t.Errorf("owner = %d (%v), want %d", got.Owner, ok, c.want)
			}
		})
	}
}

func TestPeersRosterKeepsTheSessionsWhoseClaudeCodeRuns(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Running = map[int]bool{11: true, 12: true}
	post(t, sys, 11, model.Peer{Key: "running", At: t0})
	post(t, sys, 12, model.Peer{Key: "idle-for-hours", At: t0.Add(-3 * time.Hour)})
	post(t, sys, 13, model.Peer{Key: "exited", At: t0})
	post(t, sys, 12, model.Peer{Key: "a-day-old", At: t0.Add(-25 * time.Hour)})

	got := peers(sys).Roster()
	if want := []string{"idle-for-hours", "running"}; !slices.Equal(keys(got), want) {
		t.Errorf("roster = %v, want %v", keys(got), want)
	}
	for _, gone := range []string{"exited", "a-day-old"} {
		if _, ok := sys.File(cache + "peers/" + gone + ".json"); ok {
			t.Errorf("the file of %q was not removed", gone)
		}
	}
}

func TestPeersRosterFallsBackOnTheLastPostWhereNoProcessCanBeAsked(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0) // Running is nil: Alive cannot tell, as on Windows
	post(t, sys, 11, model.Peer{Key: "recent", At: t0.Add(-9 * time.Minute)})
	post(t, sys, 12, model.Peer{Key: "quiet", At: t0.Add(-10 * time.Minute)})
	post(t, sys, 0, model.Peer{Key: "unknown-owner", At: t0.Add(-time.Minute)})

	if got, want := keys(peers(sys).Roster()), []string{"recent", "unknown-owner"}; !slices.Equal(got, want) {
		t.Errorf("roster = %v, want %v", got, want)
	}
}

func TestPeersRosterKeepsTheNewestSessionOfEachClaudeCode(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Running = map[int]bool{11: true}
	// After /clear the same Claude Code posts under a new id; the files sort
	// the old one first in one case and last in the other.
	post(t, sys, 11, model.Peer{Key: "a-before-clear", At: t0.Add(-time.Minute)})
	post(t, sys, 11, model.Peer{Key: "b-after-clear", At: t0})
	post(t, sys, 11, model.Peer{Key: "c-older-still", At: t0.Add(-2 * time.Minute)})

	if got, want := keys(peers(sys).Roster()), []string{"b-after-clear"}; !slices.Equal(got, want) {
		t.Errorf("roster = %v, want %v", got, want)
	}
	for _, gone := range []string{"a-before-clear", "c-older-still"} {
		if _, ok := sys.File(cache + "peers/" + gone + ".json"); ok {
			t.Errorf("the file of %q was not removed", gone)
		}
	}
}

func TestPeersRosterDropsAFileThatIsNotAPeer(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	if err := sys.WriteFile(cache+"peers/broken.json", []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := peers(sys).Roster(); len(got) != 0 {
		t.Errorf("roster = %+v", got)
	}
	if _, ok := sys.File(cache + "peers/broken.json"); ok {
		t.Error("the broken file was not removed")
	}
}

func TestPeersRosterNeverRemovesAPathMadeFromAFile(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Running = map[int]bool{11: true}
	outside := "/h/.claude/settings.json"
	if err := sys.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Two files of one owner, the older claiming a key that would point outside.
	data := []byte(`{"key":"../../../.claude/settings","owner":11,"at":"2026-10-03T04:00:00Z"}`)
	if err := sys.WriteFile(cache+"peers/a.json", data, 0o600); err != nil {
		t.Fatal(err)
	}
	post(t, sys, 11, model.Peer{Key: "b", At: t0})

	peers(sys).Roster()
	if _, ok := sys.File(outside); !ok {
		t.Error("a file outside the cache was removed")
	}
	if _, ok := sys.File(cache + "peers/a.json"); ok {
		t.Error("the older file was not removed")
	}
}

func TestPeersPostFailsOnAReadOnlyCache(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.ReadOnly = true
	if err := peers(sys).Post(model.Peer{Key: "s1", At: t0}); err == nil {
		t.Error("a read-only cache took the post")
	}
}
