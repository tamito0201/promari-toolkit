package model_test

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

func TestTurnTimes(t *testing.T) {
	t.Parallel()
	if _, ok := (&model.Transcript{}).TurnTimes(); ok {
		t.Error("turn times before the first turn")
	}
	tr := model.Transcript{Turns: []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 5}}
	got, ok := tr.TurnTimes()
	// Nearest rank over 11 values: the 6th for the median, the 10th for p90.
	want := model.TurnTimes{Last: 5 * time.Second, P50: 50 * time.Second, P90: 90 * time.Second}
	if !ok || got != want {
		t.Errorf("TurnTimes() = %+v, want %+v", got, want)
	}
	one := model.Transcript{Turns: []float64{7}}
	if got, _ := one.TurnTimes(); got.P50 != 7*time.Second || got.P90 != 7*time.Second {
		t.Errorf("one turn = %+v", got)
	}
}

func TestAddTurnKeepsTheLast(t *testing.T) {
	t.Parallel()
	var tr model.Transcript
	for i := range model.MaxTurnSamples + 5 {
		tr.AddTurn(time.Duration(i) * time.Second)
	}
	if len(tr.Turns) != model.MaxTurnSamples || tr.Turns[0] != 5 {
		t.Errorf("%d turns from %v", len(tr.Turns), tr.Turns[0])
	}
}

func TestAddFile(t *testing.T) {
	t.Parallel()
	var tr model.Transcript
	for _, f := range []string{"/a", "", "/a", "/b"} {
		tr.AddFile(f)
	}
	if !slices.Equal(tr.Files, []string{"/a", "/b"}) {
		t.Errorf("Files = %v", tr.Files)
	}
	for i := range model.MaxFilesKept {
		tr.AddFile("/f" + strconv.Itoa(i))
	}
	if len(tr.Files) != model.MaxFilesKept {
		t.Errorf("%d files kept", len(tr.Files))
	}
}

func TestCountTools(t *testing.T) {
	t.Parallel()
	var tr model.Transcript
	tr.CountTools([]model.ToolCount{{Name: "Read", Count: 2}, {Name: "Bash", Count: 1}}, 1)
	tr.CountTools([]model.ToolCount{{Name: "Bash", Count: 1}, {Name: "Edit", Count: 1}, {Name: "Grep", Count: 2}}, 2)
	want := model.ToolStats{Total: 7, Errors: 3, Top: []model.ToolCount{{Name: "Read", Count: 2}, {Name: "Bash", Count: 2}, {Name: "Grep", Count: 2}}}
	if tr.Tools.Total != want.Total || tr.Tools.Errors != want.Errors || !slices.Equal(tr.Tools.Top, want.Top) {
		t.Errorf("Tools = %+v; want %+v (equal counts in the order first called)", tr.Tools, want)
	}
}

func TestTranscriptRatios(t *testing.T) {
	t.Parallel()
	var none model.Transcript
	if _, ok := none.Autonomy(); ok {
		t.Error("autonomy without a prompt")
	}
	if _, ok := none.Interventions(); ok {
		t.Error("interventions without a prompt")
	}
	if _, ok := none.Tokens.CachedShare(); ok {
		t.Error("a cached share without input")
	}
	if _, ok := none.Tokens.ThinkingShare(); ok {
		t.Error("a thinking share without output")
	}
	tr := model.Transcript{
		Prompts: 4, Interrupts: 1, Denials: 2, Tools: model.ToolStats{Total: 10},
		Tokens: model.TokenTotals{Input: 10, CacheWrite: 30, CacheRead: 60, Output: 8, Thinking: 2},
	}
	if auto, _ := tr.Autonomy(); auto != 2.5 {
		t.Errorf("Autonomy = %v", auto)
	}
	if rate, _ := tr.Interventions(); rate != 75 {
		t.Errorf("Interventions = %v", rate)
	}
	if share, _ := tr.Tokens.CachedShare(); share != 60 {
		t.Errorf("CachedShare = %v", share)
	}
	if share, _ := tr.Tokens.ThinkingShare(); share != 25 {
		t.Errorf("ThinkingShare = %v", share)
	}
}

func TestCursorCounted(t *testing.T) {
	t.Parallel()
	var c model.TranscriptCursor
	if c.Counted("a") || !c.Counted("a") || c.Counted("") || c.Counted("") {
		t.Error("an id is counted once; an empty id every time")
	}
	for i := range 20 {
		c.Counted("m" + strconv.Itoa(i))
	}
	if len(c.Recent) != 16 || c.Counted("a") {
		t.Errorf("Recent = %v; the oldest id is forgotten", c.Recent)
	}
}

func TestActivityDeepWork(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	var a model.Activity
	at := start
	tick := func(d time.Duration) {
		at = at.Add(d)
		a.Observe(0, "", at)
	}
	tick(0)
	for range 25 { // a streak of 25 minutes, one render a minute
		tick(time.Minute)
	}
	tick(10 * time.Minute) // a break
	for range 5 {          // a short streak
		tick(time.Minute)
	}
	tick(6 * time.Minute) // another break
	if a.Breaks != 2 || a.DeepSeconds != (25*time.Minute).Seconds() || a.LongestSeconds != (25*time.Minute).Seconds() {
		t.Errorf("Breaks = %d, Deep = %vs, Longest = %vs", a.Breaks, a.DeepSeconds, a.LongestSeconds)
	}
	if got := a.Deep(at); got != 25*time.Minute {
		t.Errorf("Deep = %v; a fresh streak is not deep yet", got)
	}
	later := at.Add(model.DeepStreak)
	if got := a.Deep(later); got != 25*time.Minute+model.DeepStreak {
		t.Errorf("Deep = %v; the current streak counts once it is deep", got)
	}
	if got := a.Longest(later); got != 25*time.Minute {
		t.Errorf("Longest = %v", got)
	}
	if got := a.Longest(at.Add(30 * time.Minute)); got != 30*time.Minute {
		t.Errorf("Longest = %v; the current streak counts", got)
	}
}

// TestTranscriptRecordsCallsAndResults covers the rules a transcript's calls
// and results are recorded by, apart from how a transcript is written.
func TestTranscriptRecordsCallsAndResults(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	var tr model.Transcript

	tr.Called(model.ToolCall{ID: "e1", Kind: model.CallEdit, Signature: "a", File: "main.go"})
	tr.Called(model.ToolCall{ID: "e2", Kind: model.CallEdit, Signature: "b", File: "main.go"})
	if tr.Quality.Edits != 2 || tr.Quality.ReEdits != 1 || !slices.Equal(tr.Files, []string{"main.go"}) {
		t.Errorf("after two edits of one file: %+v, files %v", tr.Quality, tr.Files)
	}
	tr.Resulted(model.ToolResult{ID: "e2", Failed: true})
	if tr.Quality.EditFailures != 1 {
		t.Errorf("a failed edit was not counted: %+v", tr.Quality)
	}

	// A test run whose exit code says success while its output says failure is masked.
	tr.Called(model.ToolCall{ID: "t1", Kind: model.CallOther, Signature: "c", Command: "go test ./..."})
	tr.Resulted(model.ToolResult{ID: "t1", Output: "--- FAIL: TestX\nFAIL", At: at})
	if tr.Quality.Tests.Masked != 1 || tr.Quality.RedSince != at {
		t.Errorf("a failure behind a zero exit code: %+v", tr.Quality)
	}
	// Calls while the tests are red are counted, a subagent's are not.
	tr.Called(model.ToolCall{ID: "r1", Kind: model.CallRead, Signature: "d"})
	tr.Called(model.ToolCall{ID: "r2", Kind: model.CallRead, Signature: "e", Side: true})
	if tr.Quality.RedCalls != 1 {
		t.Errorf("red calls = %d, want 1", tr.Quality.RedCalls)
	}
	// A refused or interrupted check decides nothing.
	tr.Called(model.ToolCall{ID: "t2", Kind: model.CallOther, Signature: "f", Command: "go test ./..."})
	tr.Resulted(model.ToolResult{ID: "t2", Skipped: true, Output: "ok"})
	waiting := slices.ContainsFunc(tr.Quality.Pending, func(p model.PendingCall) bool { return p.ID == "t2" })
	if tr.Quality.RedSince != at || tr.Quality.Tests.Runs != 1 || waiting {
		t.Errorf("a skipped check changed the state: %+v", tr.Quality)
	}
	// A result nobody waited for is left alone.
	tr.Resulted(model.ToolResult{ID: "unknown", Failed: true})
	if tr.Quality.EditFailures != 1 {
		t.Errorf("an unknown result was counted: %+v", tr.Quality)
	}
}

func TestTranscriptCompactionsAndHooks(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	var tr model.Transcript
	tr.Compacted(at)
	tr.Compacted(time.Time{}) // a compaction without a readable time keeps the last one known
	if tr.Compactions != 2 || tr.LastCompaction != at {
		t.Errorf("compactions = %d, last %v", tr.Compactions, tr.LastCompaction)
	}
	tr.HookRan(false)
	tr.HookRan(true)
	if tr.Hooks != 2 || tr.HookErrors != 1 {
		t.Errorf("hooks = %d, errors %d", tr.Hooks, tr.HookErrors)
	}
}
