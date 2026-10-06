package model_test

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

// update rewrites the JSON fixtures of the model from the code as it is:
// go test ./internal/domain/model -run JSON -update
var update = flag.Bool("update", false, "rewrite the JSON fixtures of the model")

var observedAt = time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)

// step is one event of a session's tool calls.
type step func(*model.ToolObservations)

func called(c model.ToolCall) step     { return func(o *model.ToolObservations) { o.Called(c) } }
func resulted(r model.ToolResult) step { return func(o *model.ToolObservations) { o.Resulted(r) } }

// repeated returns n steps, each made from its index.
func repeated(n int, each func(i int) []step) []step {
	steps := make([]step, 0, n)
	for i := range n {
		steps = append(steps, each(i)...)
	}
	return steps
}

func observe(steps ...step) model.ToolObservations {
	var o model.ToolObservations
	for _, s := range steps {
		s(&o)
	}
	return o
}

// pendingNamed are 300 calls of 300 tools that wait for their results: more
// than the ids kept, the calls awaited and the tool names tracked.
func pendingNamed() []step {
	return repeated(300, func(i int) []step {
		return []step{called(model.ToolCall{ID: "pending" + strconv.Itoa(i), Name: "tool" + strconv.Itoa(i), At: observedAt})}
	})
}

// firstNames counts one call of each of the first n tools.
func firstNames(n int) map[string]int {
	names := map[string]int{}
	for i := range n {
		names["tool"+strconv.Itoa(i)] = 1
	}
	return names
}

func TestToolObservationsView(t *testing.T) {
	t.Parallel()
	bounded := firstNames(model.ToolKindsLimit)
	bounded["tool1"] = 2
	window := make([]float64, model.MaxTurnSamples)
	for i := range window {
		window[i] = float64(i + 300 - model.MaxTurnSamples)
	}
	tests := []struct {
		name  string
		steps []step
		want  model.ToolObservationsView
	}{
		{"nothing observed", nil, model.ToolObservationsView{}},
		{
			"parallel calls pair with their results by id, and a repeat counts once",
			[]step{
				called(model.ToolCall{ID: "a", Name: "Read", At: observedAt}),
				called(model.ToolCall{ID: "a", Name: "Read", At: observedAt}),
				called(model.ToolCall{ID: "b", Name: "Bash", At: observedAt.Add(time.Second)}),
				called(model.ToolCall{ID: "side", Name: "Bash", Side: true}),
				called(model.ToolCall{Name: "Read"}),
				resulted(model.ToolResult{ID: "b", Failed: true, At: observedAt.Add(3 * time.Second)}),
				resulted(model.ToolResult{ID: "a", At: observedAt.Add(4 * time.Second)}),
				resulted(model.ToolResult{ID: "a", Failed: true, At: observedAt.Add(5 * time.Second)}),
				resulted(model.ToolResult{ID: "side", Side: true}),
				resulted(model.ToolResult{}),
				resulted(model.ToolResult{ID: "orphan"}),
			},
			model.ToolObservationsView{
				Calls: 2, Completed: 2, Failed: 1, MissingID: 1, Unpaired: 1, Switches: 1, Transitions: 1,
				Names: map[string]int{"Read": 1, "Bash": 1}, Seconds: []float64{2, 4},
			},
		},
		{
			"a refused or interrupted call is skipped, not failed",
			[]step{
				called(model.ToolCall{ID: "a", Name: "Bash", At: observedAt}),
				called(model.ToolCall{ID: "denied", Name: "Bash", At: observedAt}),
				resulted(model.ToolResult{ID: "denied", Skipped: true, Failed: true}),
			},
			model.ToolObservationsView{Calls: 2, Skipped: 1, Pending: 1, Transitions: 1, Names: map[string]int{"Bash": 2}},
		},
		{
			"a missing or reversed time is untimed, and zero seconds is a measurement",
			repeated(4, func(i int) []step {
				pairs := [][2]time.Time{{{}, observedAt}, {observedAt, {}}, {observedAt, observedAt.Add(-time.Second)}, {observedAt, observedAt}}
				id := strconv.Itoa(i)
				return []step{called(model.ToolCall{ID: id, At: pairs[i][0]}), resulted(model.ToolResult{ID: id, At: pairs[i][1]})}
			}),
			model.ToolObservationsView{Calls: 4, Completed: 4, Untimed: 3, Seconds: []float64{0}},
		},
		{
			"the calls awaited and the tool names tracked are bounded, and a known name past the bound still counts",
			append(pendingNamed(), called(model.ToolCall{ID: "known-kind", Name: "tool1", At: observedAt})),
			model.ToolObservationsView{
				Calls: 301, Pending: model.ToolObservationLimit, Evicted: 301 - model.ToolObservationLimit,
				Names: bounded, OtherNames: 300 - model.ToolKindsLimit, Switches: 300, Transitions: 300,
			},
		},
		{
			"the round trips kept are the last",
			repeated(300, func(i int) []step {
				id := "paired" + strconv.Itoa(i)
				return []step{called(model.ToolCall{ID: id, At: observedAt}), resulted(model.ToolResult{ID: id, At: observedAt.Add(time.Duration(i) * time.Second)})}
			}),
			model.ToolObservationsView{Calls: 300, Completed: 300, Seconds: window},
		},
		{
			"a call id forgotten past the bound is a new call, a recent one is a repeat",
			append(pendingNamed()[:model.ToolObservationLimit+1],
				called(model.ToolCall{ID: "pending0", At: observedAt}),
				called(model.ToolCall{ID: "pending256", At: observedAt})),
			model.ToolObservationsView{
				Calls: model.ToolObservationLimit + 2, Pending: model.ToolObservationLimit, Evicted: 2,
				Names: firstNames(model.ToolKindsLimit), OtherNames: model.ToolObservationLimit + 1 - model.ToolKindsLimit,
				Switches: model.ToolObservationLimit, Transitions: model.ToolObservationLimit,
			},
		},
		{
			"a result id forgotten past the bound is unpaired, a recent one is a repeat",
			append(repeated(model.ToolObservationLimit+1, func(i int) []step {
				id := "r" + strconv.Itoa(i)
				return []step{called(model.ToolCall{ID: id}), resulted(model.ToolResult{ID: id})}
			}), resulted(model.ToolResult{ID: "r0"}), resulted(model.ToolResult{ID: "r256"})),
			model.ToolObservationsView{Calls: 257, Completed: 257, Untimed: 257, Unpaired: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			o := observe(tt.steps...)
			if got := o.View(); !equalView(got, tt.want) {
				t.Errorf("View() = %+v\nwant     %+v", got, tt.want)
			}
		})
	}
}

// equalView compares two views; an empty map or slice is the same as none.
func equalView(a, b model.ToolObservationsView) bool {
	if !maps.Equal(a.Names, b.Names) || !slices.Equal(a.Seconds, b.Seconds) {
		return false
	}
	a.Names, b.Names, a.Seconds, b.Seconds = nil, nil, nil, nil
	return reflect.DeepEqual(a, b)
}

func TestToolObservationsViewIsACopy(t *testing.T) {
	t.Parallel()
	o := observe(called(model.ToolCall{ID: "a", Name: "Read", At: observedAt}), resulted(model.ToolResult{ID: "a", At: observedAt.Add(time.Second)}))
	tests := []struct {
		name   string
		change func(*model.ToolObservationsView)
	}{
		{"the names", func(v *model.ToolObservationsView) { v.Names["Read"] = 99 }},
		{"the round trips", func(v *model.ToolObservationsView) { v.Seconds[0] = 99 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := o.View()
			tt.change(&v)
			if got := o.View(); got.Names["Read"] != 1 || got.Seconds[0] != 1 {
				t.Errorf("changing the view's %s changed the observations: %+v", tt.name, got)
			}
		})
	}
}

// observedSession is a session's tool calls that touches every counter of
// ToolObservations: paired, failed, skipped, unpaired, untimed, evicted and
// unnamed calls, and more tool names than are tracked.
func observedSession() model.ToolObservations {
	return observe(append([]step{
		called(model.ToolCall{ID: "a", Name: "Read", At: observedAt}),
		called(model.ToolCall{ID: "b", Name: "Bash", At: observedAt.Add(time.Second)}),
		called(model.ToolCall{Name: "Read"}),
		called(model.ToolCall{ID: "denied", Name: "Bash", At: observedAt}),
		called(model.ToolCall{ID: "untimed", Name: "Edit"}),
		resulted(model.ToolResult{ID: "b", Failed: true, At: observedAt.Add(3 * time.Second)}),
		resulted(model.ToolResult{ID: "a", At: observedAt.Add(4 * time.Second)}),
		resulted(model.ToolResult{ID: "denied", Skipped: true}),
		resulted(model.ToolResult{ID: "untimed", At: observedAt}),
		resulted(model.ToolResult{ID: "orphan"}),
	}, repeated(model.ToolObservationLimit+4, func(i int) []step {
		return []step{called(model.ToolCall{ID: "p" + strconv.Itoa(i), Name: "tool" + strconv.Itoa(i%130), At: observedAt})}
	})...)...)
}

func indented(t *testing.T, v any) []byte {
	t.Helper()
	out, err := json.Marshal(v, json.Deterministic(true), jsontext.Multiline(true), jsontext.WithIndent("  "))
	if err != nil {
		t.Fatal(err)
	}
	return append(out, '\n')
}

// TestToolObservationsJSON pins the stored form: testdata/tool_observations.json
// was written by the version whose fields were public, so a cache written
// before the fields were hidden is read as it was.
func TestToolObservationsJSON(t *testing.T) {
	t.Parallel()
	fixture := filepath.Join("testdata", "tool_observations.json")
	if *update {
		if err := os.WriteFile(fixture, indented(t, observedSession()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var restored model.ToolObservations
	if err := json.Unmarshal(stored, &restored); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		v    model.ToolObservations
	}{
		{"the session as observed", observedSession()},
		{"the session read back", restored},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := indented(t, tt.v); !bytes.Equal(got, stored) {
				t.Errorf("the JSON differs from %s:\n%s", fixture, got)
			}
		})
	}
	t.Run("the session read back goes on as the session", func(t *testing.T) {
		t.Parallel()
		next := []step{called(model.ToolCall{ID: "p259", At: observedAt}), resulted(model.ToolResult{ID: "p259", At: observedAt.Add(time.Second)})}
		live, back := observedSession(), restored
		for _, s := range next {
			s(&live)
			s(&back)
		}
		if !reflect.DeepEqual(live.View(), back.View()) {
			t.Errorf("read back %+v\nlive      %+v", back.View(), live.View())
		}
	})
}

// TestToolObservationsCopyIsIndependent advances a copy: a copy shares the
// maps and slices of the value, so advancing it must not write through them.
func TestToolObservationsCopyIsIndependent(t *testing.T) {
	t.Parallel()
	original := observedSession()
	want := indented(t, original)
	copied := original
	for _, s := range []step{
		called(model.ToolCall{ID: "p259", Name: "Read", At: observedAt}),
		resulted(model.ToolResult{ID: "p259", At: observedAt.Add(time.Second)}),
		called(model.ToolCall{ID: "fresh", Name: "Read", At: observedAt}),
		resulted(model.ToolResult{ID: "fresh", At: observedAt.Add(time.Second)}),
	} {
		s(&copied)
	}
	if got := indented(t, original); !bytes.Equal(got, want) {
		t.Errorf("advancing a copy changed the original:\n%s", got)
	}
}

func TestToolObservationsRefuseBrokenJSON(t *testing.T) {
	t.Parallel()
	ids := func(n int) string {
		return joined("[", n, func(i int) string { return `"x` + strconv.Itoa(i) + `"` }, "]")
	}
	names := joined("{", model.ToolKindsLimit+1, func(i int) string { return `"t` + strconv.Itoa(i) + `":1` }, "}")
	tests := []struct {
		name, json string
		broken     bool
	}{
		{"consistent", `{"calls":3,"completed":1,"failed":1,"skipped":1,"evicted":1,"untimed":1,"transitions":1,"switches":1}`, false},
		{"empty", `{}`, false},
		{"calls that do not add up", `{"calls":2,"completed":1}`, true},
		{"more failures than results", `{"calls":1,"completed":1,"failed":2}`, true},
		{"more timings than results", `{"calls":1,"completed":1,"untimed":1,"seconds":[1]}`, true},
		{"more switches than transitions", `{"switches":1}`, true},
		{"a negative count", `{"calls":-1,"evicted":-1}`, true},
		{"more pending calls than are kept", `{"calls":257,"pending":` + joined("[", 257, func(int) string { return `{"id":"x"}` }, "]") + `}`, true},
		{"more call ids than are kept", `{"recent":` + ids(257) + `}`, true},
		{"more result ids than are kept", `{"result_ids":` + ids(257) + `}`, true},
		{"more tool names than are tracked", `{"names":` + names + `}`, true},
		{"more round trips than are kept", `{"calls":201,"completed":201,"seconds":` + joined("[", 201, func(int) string { return "1" }, "]") + `}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var o model.ToolObservations
			err := json.Unmarshal([]byte(tt.json), &o)
			if got := errors.Is(err, model.ErrBrokenObservations); got != tt.broken || !tt.broken && err != nil {
				t.Errorf("Unmarshal(%s) = %v; broken %v", tt.name, err, tt.broken)
			}
			// A transcript holding broken observations is not read back at all.
			var tr model.Transcript
			if err := json.Unmarshal([]byte(`{"observations":`+tt.json+`}`), &tr); (err != nil) != tt.broken {
				t.Errorf("a transcript with %s: %v", tt.name, err)
			}
		})
	}
	t.Run("a value of another type", func(t *testing.T) {
		t.Parallel()
		var o model.ToolObservations
		if err := json.Unmarshal([]byte(`{"calls":"x"}`), &o); err == nil || errors.Is(err, model.ErrBrokenObservations) {
			t.Errorf("err = %v, want a decoding error", err)
		}
	})
}

// joined writes n items between open and closing, separated by commas.
func joined(open string, n int, item func(i int) string, closing string) string {
	parts := make([]string, n)
	for i := range n {
		parts[i] = item(i)
	}
	return open + strings.Join(parts, ",") + closing
}
