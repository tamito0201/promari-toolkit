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
		{"sh tools/run.sh task ci > log", model.CheckTest},
		{"node --test", model.CheckTest},
		{"node scripts/fresh.test.mjs", model.CheckTest},
		{"node build.js", model.CheckNone},
		{"GOFLAGS=-count=1 CI=true go test ./...", model.CheckTest},
		{"mise run test", model.CheckNone}, // a task named test is not a runner
		{"sh tools/run.sh task lint", model.CheckBuild},
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
	q.Checked(model.CheckTest, model.OutcomePass, false, t0.Add(12*time.Minute))
	if q.Unverified != nil || !q.RedSince.IsZero() {
		t.Errorf("after a pass: Unverified = %v, RedSince = %v", q.Unverified, q.RedSince)
	}
	if !q.GreenSince.Equal(t0.Add(12*time.Minute)) || q.LastRepair != 12*time.Minute {
		t.Errorf("the repair: GreenSince = %v, LastRepair = %v", q.GreenSince, q.LastRepair)
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

func TestResearchMarkers(t *testing.T) {
	t.Parallel()
	for _, line := range []string{"m := mocks.NewStore(t)", "jest.fn()", "vi.fn()", "sinon.stub(a)", "@patch('x')", "monkeypatch.setattr", "fakeClock := x", "spyOn(a)"} {
		if !model.MarksMock(line) {
			t.Errorf("MarksMock(%q) = false", line)
		}
	}
	for _, line := range []string{"got := add(1, 2)", "hammock := 1"} {
		if model.MarksMock(line) {
			t.Errorf("MarksMock(%q) = true", line)
		}
	}
	for _, c := range []struct {
		file, line string
		want       bool
	}{
		{"go.mod", "\tgithub.com/x/y v1.2.3", true},
		{"go.mod", "require golang.org/x/text v0.20.0", true},
		{"go.mod", "go 1.27", false},
		{"web/package.json", `  "left-pad": "^1.3.0",`, true},
		{"package.json", `  "@scope/pkg": "workspace:*",`, true},
		{"package.json", `  "version": "1.5.0",`, false},
		{"package.json", `  "node": ">=20",`, false},
		{"package.json", `  "description": "a tool",`, false},
		{"requirements-dev.txt", "requests==2.32.0", true},
		{"requirements.txt", "# a comment", false},
		{"pyproject.toml", `  "httpx>=0.27",`, true},
		{"Cargo.toml", `serde = { version = "1", features = ["derive"] }`, true},
		{"Cargo.toml", `version = "0.1.0"`, false},
		{"Gemfile", `gem "rails"`, true},
		{"go.sum", "github.com/x/y v1.2.3 h1:abc", false},
	} {
		if got := model.AddsDependency(c.file, c.line); got != c.want {
			t.Errorf("AddsDependency(%q, %q) = %v", c.file, c.line, got)
		}
	}
	if !model.IsAICoauthor("Claude <claude@example.com>") || !model.IsAICoauthor("A <a@x>,Copilot <c@x>") || model.IsAICoauthor("Alice <a@x>") {
		t.Error("IsAICoauthor")
	}
	for _, line := range []string{`t.Skip("x")`, "@pytest.mark.skip", "@pytest.mark.xfail(reason='x')", "it.skip('x', () => {})", "xit('x')", "@Disabled", "#[ignore]"} {
		if !model.MarksSkip(line) {
			t.Errorf("MarksSkip(%q) = false", line)
		}
	}
	if model.MarksSkip("skipped := 0") || !model.MarksAssert("assert.Equal(t, 1, got)") || !model.MarksAssert(`t.Errorf("x")`) || !model.MarksAssert("expect(a).toBe(1)") || model.MarksAssert("got := 1") {
		t.Error("MarksSkip / MarksAssert")
	}
}

func TestClaims(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"All tests pass now.", "tests passed", "テストはすべて通りました", "テストが成功しました"} {
		if !model.ClaimsTests(s) {
			t.Errorf("ClaimsTests(%q) = false", s)
		}
	}
	for _, s := range []string{"I will run the tests", "CI は通りました", "テストを書きます"} {
		if model.ClaimsTests(s) {
			t.Errorf("ClaimsTests(%q) = true", s)
		}
	}
	if !model.ClaimsDone("修正しました") || !model.ClaimsDone("I fixed the cause.") || model.ClaimsDone("a fixed path") || model.ClaimsDone("レビューが完了しました") || model.ClaimsDone("これから修正します") {
		t.Error("ClaimsDone")
	}

	var q model.Quality
	q.Claimed("tests pass") // no test run recognised yet: nothing to tell it from the truth
	q.Checked(model.CheckTest, model.OutcomeUnknown, false, t0)
	q.Claimed("tests pass") // the last run ended unknown: the agent may have read an empty output right
	q.Claimed("修正しました")     // fixed while nothing failed: no claim
	q.Checked(model.CheckTest, model.OutcomePass, false, t0)
	q.Claimed("tests pass") // verified
	q.Edited("/w/a.go")
	q.Claimed("tests pass") // an edit since the last run: a claim
	q.Checked(model.CheckTest, model.OutcomeFail, false, t0)
	q.Claimed("tests pass") // the last run failed: a claim
	q.Claimed("修正しました")     // fixed while the last run failed: a claim
	if q.Claims != 3 {
		t.Errorf("Claims = %d, want 3", q.Claims)
	}
}

func TestTrace(t *testing.T) {
	t.Parallel()
	var tr model.Trace
	for _, c := range []struct {
		kind int
		sig  string
	}{
		{model.CallRead, "a"},
		{model.CallRead, "a"},
		{model.CallSearch, "g"},
		{model.CallSearch, "g"},
		{model.CallOther, "x"},
		{model.CallEdit, "e"},
		{model.CallRead, "a"}, // an edit forgets the reads
		{model.CallEdit, "e1"},
		{model.CallEdit, "e2"},
		{model.CallOther, "b"},
		{model.CallEdit, "e3"},
		{model.CallRead, ""},
		{model.CallRead, ""},
	} {
		tr.Called(c.kind, c.sig)
	}
	if tr.Explores != 7 || tr.Rereads != 1 || tr.Researches != 1 || tr.MaxEditRun != 2 || tr.EditRun != 0 {
		t.Errorf("Trace = %+v", tr)
	}
	if r, ok := tr.ExploreRatio(3); !ok || math.Abs(r-7.0/3) > 1e-9 {
		t.Errorf("ExploreRatio = %v, %v", r, ok)
	}
	if _, ok := tr.ExploreRatio(0); ok {
		t.Error("no edit, no ratio")
	}
	var full model.Trace
	for i := range 250 {
		full.Called(model.CallRead, string(rune('A'+i)))
	}
	if len(full.Seen) != 200 {
		t.Errorf("Seen = %d, bounded", len(full.Seen))
	}

	tr.Observed(400)
	tr.Observed(4000)
	if all, largest := tr.ObservedTokens(); all != 1100 || largest != 1000 {
		t.Errorf("ObservedTokens = %v, %v", all, largest)
	}
	tr.Prompt(100)
	tr.Prompt(100) // cost nothing: no turn
	if _, _, ok := tr.TurnSpread(); ok {
		t.Error("no prompt ended yet")
	}
	tr.Prompt(300)
	tr.Prompt(350)
	tr.Prompt(1350)
	if median, largest, ok := tr.TurnSpread(); !ok || median != 200 || largest != 1000 || tr.PromptsSinceCompact != 5 {
		t.Errorf("TurnSpread = %v, %v, %v; since compact %d", median, largest, ok, tr.PromptsSinceCompact)
	}
	tr.Compacted()
	if tr.ObservedBytes != 0 || tr.LargestObserved != 0 || tr.PromptsSinceCompact != 0 {
		t.Errorf("after a compaction: %+v", tr)
	}
	var many model.Trace
	for i := range 210 {
		many.Prompt(float64(i))
	}
	if len(many.TurnTokens) != 200 {
		t.Errorf("TurnTokens = %d, bounded", len(many.TurnTokens))
	}
}

func TestQualityRepairOnlyAfterRed(t *testing.T) {
	t.Parallel()
	var q model.Quality
	q.Checked(model.CheckTest, model.OutcomePass, false, t0)
	if !q.GreenSince.IsZero() {
		t.Error("a pass without a failure before it repairs nothing")
	}
	q.Checked(model.CheckTest, model.OutcomeFail, false, t0.Add(time.Minute))
	q.Checked(model.CheckTest, model.OutcomePass, false, t0.Add(4*time.Minute))
	q.Checked(model.CheckTest, model.OutcomePass, false, t0.Add(9*time.Minute))
	if !q.GreenSince.Equal(t0.Add(4*time.Minute)) || q.LastRepair != 3*time.Minute {
		t.Errorf("GreenSince = %v, LastRepair = %v; a pass after a pass keeps the repair", q.GreenSince, q.LastRepair)
	}
}
