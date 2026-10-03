package state_test

import (
	"errors"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/filecache"
	"promari-statusline/internal/infrastructure/platform/platformtest"
	"promari-statusline/internal/infrastructure/state"
)

var t0 = time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC)

const cache = "/h/.cache/promari-statusline/"

func TestActivities(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	activities := state.Activities{Store: filecache.NewStore(sys)}

	if got := activities.Load("s1"); got.Turns != 0 || len(got.Samples) != 0 {
		t.Errorf("an unknown session has activity %+v", got)
	}
	var a model.Activity
	a.Observe(1000, "p1", t0)
	if err := activities.Save("s1", a); err != nil {
		t.Fatal(err)
	}
	if got := activities.Load("s1"); got.Turns != 1 || !got.LastSeen.Equal(t0) {
		t.Errorf("Load = %+v", got)
	}
	if got := activities.Load("s2"); got.Turns != 0 {
		t.Errorf("another session sees %+v", got)
	}
	if _, ok := sys.File(cache + "sessions/s1.json"); !ok {
		t.Errorf("the activity is not in a file named after the session: %v", sys.Glob(cache+"*/*"))
	}
}

func TestLimits(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	limits := state.Limits{Store: filecache.NewStore(sys)}

	if _, _, err := limits.Last(); !errors.Is(err, repository.ErrNone) {
		t.Errorf("Last() with nothing remembered: %v", err)
	}
	if got := limits.History(); len(got) != 0 {
		t.Errorf("History() with nothing recorded: %v", got)
	}

	want := model.RateLimits{FiveHour: model.Some(model.RateWindow{UsedPct: 0, ResetsAt: t0.Add(time.Hour)})}
	if err := limits.Remember(want, t0); err != nil {
		t.Fatal(err)
	}
	got, at, err := limits.Last()
	if err != nil || !at.Equal(t0) {
		t.Fatalf("Last() = %+v, %v, %v", got, at, err)
	}
	// Zero percent used is a known window: it must survive the round trip.
	if w, ok := got.FiveHour.Get(); !ok || w.UsedPct != 0 || !w.ResetsAt.Equal(t0.Add(time.Hour)) || got.SevenDay.Present() {
		t.Errorf("remembered %+v", got)
	}

	if err := limits.Remember(model.RateLimits{}, t0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := limits.Last(); !errors.Is(err, repository.ErrNone) {
		t.Errorf("empty limits were remembered as limits: %v", err)
	}

	history := model.RateHistory{}.Record(want, t0)
	if err := limits.SaveHistory(history); err != nil {
		t.Fatal(err)
	}
	if back := limits.History(); len(back) != 1 || back[0].FiveHour.Or(-1) != 0 || back[0].SevenDay.Present() {
		t.Errorf("History() = %+v", back)
	}
}

func TestRecorder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"the input as it arrived", `{"version":"2.1.34"}`, `{"version":"2.1.34"}`},
		{"no input is an empty object", "", "{}"},
		{"input that is not JSON is kept as evidence", "not json", "not json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			recorder := state.Recorder{Store: filecache.NewStore(sys)}
			recorder.Input([]byte(tt.raw))
			recorder.Width("COLUMNS", 85)
			if got, _ := sys.File(cache + "last-input.json"); got != tt.want {
				t.Errorf("last input = %q, want %q", got, tt.want)
			}
			if got, _ := sys.File(cache + "width.txt"); got != "COLUMNS 85\n" {
				t.Errorf("width = %q", got)
			}
		})
	}
	t.Run("a cache that cannot be written does not stop a render", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		sys.ReadOnly = true
		recorder := state.Recorder{Store: filecache.NewStore(sys)}
		recorder.Input([]byte("{}"))
		recorder.Width("tty", 98)
	})
}

func TestSwitches(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	switches := state.Switches{Store: filecache.NewStore(sys)}
	if switches.BlinkDemo() {
		t.Error("the blink demo is on without its file")
	}
	sys.Files[cache+"blink-demo"] = nil
	if !switches.BlinkDemo() {
		t.Error("the blink demo is off although its file exists")
	}
}
