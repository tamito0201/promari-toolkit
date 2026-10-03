package model_test

import (
	"math"
	"slices"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

func TestClassifyCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		command string
		want    model.CheckKind
	}{
		{"go test ./...", model.CheckTest},
		{"cd /w && go test ./... 2>&1 | tail -20", model.CheckTest},
		{"go build ./... && go test ./x", model.CheckTest}, // the tests build first
		{"python3 -m pytest -q", model.CheckTest},
		{"pnpm --filter web test", model.CheckNone}, // a filter between the manager and the script is not recognised
		{"pnpm test", model.CheckTest},
		{"npx vitest run", model.CheckTest},
		{"cargo nextest run", model.CheckTest},
		{"task ci", model.CheckTest},
		{"time make check", model.CheckTest},
		{"go vet ./... && golangci-lint run", model.CheckBuild},
		{"npx tsc --noEmit", model.CheckBuild},
		{"pnpm run lint", model.CheckBuild},
		{"ruff check .", model.CheckBuild},
		{"git log --grep 'go test'", model.CheckNone},
		{"grep -rn 'pytest' docs", model.CheckNone},
		{"ls", model.CheckNone},
	}
	for _, tt := range tests {
		if got := model.ClassifyCommand(tt.command); got != tt.want {
			t.Errorf("ClassifyCommand(%q) = %v, want %v", tt.command, got, tt.want)
		}
	}
}

func TestJudgeCheck(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		kind       model.CheckKind
		command    string
		exitFailed bool
		output     string
		want       model.Outcome
	}{
		{"an exit status that failed", model.CheckTest, "go test ./...", true, "", model.OutcomeFail},
		{"a Go failure hidden by a pipe", model.CheckTest, "go test ./... | tail", false, "--- FAIL: TestX (0.00s)\nFAIL\n", model.OutcomeFail},
		{"a Go package that failed", model.CheckTest, "go test ./... | grep -v ok", false, "FAIL\tx/y\t0.1s\n", model.OutcomeFail},
		{"a panic", model.CheckTest, "go test | tail", false, "panic: boom\n", model.OutcomeFail},
		{"pytest's summary", model.CheckTest, "pytest | tail -3", false, "==== 2 failed, 10 passed in 1.2s ====\n", model.OutcomeFail},
		{"jest's summary", model.CheckTest, "npx jest | tail", false, "Tests:       1 failed, 4 passed, 5 total\n", model.OutcomeFail},
		{"vitest's summary", model.CheckTest, "vitest run | tail", false, " Test Files  1 failed | 3 passed (4)\n", model.OutcomeFail},
		{"cargo's summary", model.CheckTest, "cargo test | tail", false, "test result: FAILED. 3 passed; 1 failed\n", model.OutcomeFail},
		{"mocha's summary", model.CheckTest, "mocha | tail", false, "  3 passing\n  1 failing\n", model.OutcomeFail},
		{"Go passed through a pipe", model.CheckTest, "go test ./... | tail -3", false, "ok  \tx/y\t0.2s\n", model.OutcomePass},
		{"pytest passed", model.CheckTest, "pytest | tail -1", false, "==== 12 passed in 0.5s ====\n", model.OutcomePass},
		{"unittest passed", model.CheckTest, "python -m unittest | tail", false, "Ran 3 tests\n\nOK\n", model.OutcomePass},
		{"a direct run that exited with 0", model.CheckTest, "go test ./...", false, "", model.OutcomePass},
		{"a piped run whose output says nothing", model.CheckTest, "go test ./... | tail -1", false, "coverage: 98%\n", model.OutcomeUnknown},
		{"an or is not a pipe", model.CheckTest, "go test ./... || true", false, "", model.OutcomePass},
		{"a compile error hidden by a pipe", model.CheckBuild, "go build ./... | head", false, "a/b.go:12:3: undefined: x\n", model.OutcomeFail},
		{"a type error", model.CheckBuild, "tsc | head", false, "src/a.ts(3,1): error TS2304: x\n", model.OutcomeFail},
		{"lint issues", model.CheckBuild, "golangci-lint run | tail", false, "3 issues:\n", model.OutcomeFail},
		{"a quiet build passed, pipe or not", model.CheckBuild, "go build ./... | head", false, "", model.OutcomePass},
	}
	for _, tt := range tests {
		if got := model.JudgeCheck(tt.kind, tt.command, tt.exitFailed, tt.output); got != tt.want {
			t.Errorf("%s: JudgeCheck = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestQualityRecords(t *testing.T) {
	t.Parallel()
	var q model.Quality

	// Repeats are calls identical to the one just before; an empty signature never repeats.
	for _, s := range []string{"a", "a", "b", "a", "", ""} {
		q.Call(s)
	}
	if q.Repeats != 1 {
		t.Errorf("Repeats = %d, want 1", q.Repeats)
	}

	// Edits: a failed edit starts a streak, a successful one ends it; only source files await the tests.
	q.EditFailed()
	q.EditFailed()
	if q.EditFailures != 2 || q.EditFailStreak != 2 {
		t.Errorf("EditFailures, Streak = %d, %d", q.EditFailures, q.EditFailStreak)
	}
	for _, f := range []string{"/w/a.go", "/w/a.go", "/w/README.md", "/w/b.py"} {
		q.Edited(f)
	}
	if q.EditFailStreak != 0 || !slices.Equal(q.Unverified, []string{"/w/a.go", "/w/b.py"}) {
		t.Errorf("Streak = %d, Unverified = %v", q.EditFailStreak, q.Unverified)
	}

	// Tests stay red from the first failure; a pass verifies the edits and ends the red.
	q.Checked(model.CheckTest, model.OutcomeFail, true, t0)
	q.Checked(model.CheckTest, model.OutcomeFail, false, t0.Add(time.Minute))
	if !q.RedSince.Equal(t0) || q.Tests.Masked != 1 || q.Tests.Last != model.OutcomeFail {
		t.Errorf("RedSince = %v, Tests = %+v", q.RedSince, q.Tests)
	}
	q.Checked(model.CheckTest, model.OutcomeUnknown, false, t0)
	if len(q.Unverified) != 2 || q.Tests.Last != model.OutcomeUnknown {
		t.Error("a run of unknown outcome verifies nothing")
	}
	q.Checked(model.CheckTest, model.OutcomePass, false, t0)
	if q.Unverified != nil || !q.RedSince.IsZero() {
		t.Errorf("after a pass: Unverified = %v, RedSince = %v", q.Unverified, q.RedSince)
	}
	if rate, ok := q.Tests.FailRate(); !ok || math.Abs(rate-200.0/3) > 0.01 {
		t.Errorf("FailRate = %v, %v; the unknown run is not counted", rate, ok)
	}
	q.Checked(model.CheckBuild, model.OutcomePass, false, t0)
	q.Checked(model.CheckNone, model.OutcomePass, false, t0)
	if q.Builds.Runs != 1 || q.Tests.Runs != 4 {
		t.Errorf("Builds = %+v, Tests = %+v", q.Builds, q.Tests)
	}
	if _, ok := (model.CheckRuns{Runs: 1}).FailRate(); ok {
		t.Error("no decided run has no fail rate")
	}

	// Calls wait for their results; the oldest are dropped past the bound.
	for i := range 40 {
		q.Await(model.PendingCall{ID: string(rune('A' + i))})
	}
	if len(q.Pending) != 32 {
		t.Errorf("Pending = %d", len(q.Pending))
	}
	if _, ok := q.Resolve("A"); ok {
		t.Error("the oldest call was dropped")
	}
	if p, ok := q.Resolve(string(rune('A' + 39))); !ok || len(q.Pending) != 31 || p.ID != string(rune('A'+39)) {
		t.Errorf("Resolve = %+v, %v", p, ok)
	}

	// The unverified files are bounded.
	var many model.Quality
	for i := range model.MaxUnverified + 5 {
		many.Edited("/w/" + string(rune('A'+i%26)) + string(rune('a'+i/26)) + ".go")
	}
	if len(many.Unverified) != model.MaxUnverified {
		t.Errorf("Unverified = %d", len(many.Unverified))
	}
}

func TestPaths(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"a/b_test.go", "tests/x.py", "test_x.py", "src/a.test.ts", "src/a.spec.jsx", "__tests__/a.js", "src/FooTest.java", "e2e/run.go", "Test/x.cs"} {
		if !model.IsTestPath(p) {
			t.Errorf("IsTestPath(%q) = false", p)
		}
	}
	for _, p := range []string{"a/b.go", "latest.go", "src/Latest.java", "contest/x.go", "attest.py"} {
		if model.IsTestPath(p) {
			t.Errorf("IsTestPath(%q) = true", p)
		}
	}
	if !model.IsSourcePath("a/B.GO") || model.IsSourcePath("README.md") || model.IsSourcePath("Makefile") {
		t.Error("IsSourcePath")
	}
}

func TestChangeMeasures(t *testing.T) {
	t.Parallel()
	// The example of Kamei et al. 2013: 30, 20 and 10 lines over three files, entropy 1.46 bits.
	changes := []model.FileChange{
		{Path: "a/x.go", Added: 30},
		{Path: "a/y_test.go", Added: 10, Deleted: 10},
		{Path: "b/c/z.go", Deleted: 10},
		{Path: "docs/r.md", Added: 500},
		{Path: "img.png"},
	}
	s, ok := model.ChangeSpread(changes)
	if !ok || s.Files != 3 || s.Dirs != 2 || s.Subsystems != 2 || math.Abs(s.Entropy-1.459/math.Log2(3)) > 0.001 {
		t.Errorf("ChangeSpread = %+v, %v", s, ok)
	}
	if one, _ := model.ChangeSpread(changes[:1]); one.Entropy != 0 {
		t.Errorf("one file has no entropy: %+v", one)
	}
	if share, ok := model.TestShare(changes); !ok || math.Abs(share-100.0/3) > 0.01 {
		t.Errorf("TestShare = %v, %v; documents are left out", share, ok)
	}
	docs := changes[3:]
	if _, ok := model.ChangeSpread(docs); ok {
		t.Error("a change of documents has no spread")
	}
	if _, ok := model.TestShare(docs); ok {
		t.Error("a change of documents has no test share")
	}
	if !model.MarksDebt("+ // TODO: later") || !model.MarksDebt("# FIXME") || model.MarksDebt("todos := 1") || model.MarksDebt("XXXL") {
		t.Error("MarksDebt")
	}
	for subject, want := range map[string]bool{
		"fix: x": true, "fix(statusline): x": true, "Fix!: x": true, `Revert "feat: x"`: true, "revert: x": true, "hotfix: x": true,
		"feat: fix the prefix": false, "docs: x": false, "fixture: x": false,
	} {
		if got := model.IsFixCommit(subject); got != want {
			t.Errorf("IsFixCommit(%q) = %v", subject, got)
		}
	}
	if (model.FileChange{Added: 2, Deleted: 3}).Lines() != 5 {
		t.Error("Lines")
	}
}
