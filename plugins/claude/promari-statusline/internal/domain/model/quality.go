package model

import (
	"math"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
)

// CheckKind is the kind of a shell command whose outcome says something about the
// quality of the work: a test run, or a build, type check or lint.
type CheckKind int8

// The kinds of checks.
const (
	CheckNone CheckKind = iota
	CheckTest
	CheckBuild
)

// Outcome is how a check ended.
type Outcome int8

// The outcomes of a check. Unknown is a run whose output was cut and whose exit
// status was hidden by a pipe: nothing says how it ended.
const (
	OutcomeUnknown Outcome = iota
	OutcomePass
	OutcomeFail
)

// The commands that run tests, and those that build, type check or lint. A
// runner is recognised at the start of the command or after a separator, so
// "cd x && go test ./..." is a test and "git log --grep 'go test'" is not.
var (
	testCommand  = regexp.MustCompile(`(?:^|[;&|(\n]\s*|\btime\s+|\brun\.sh\s+|\bmise (?:run|exec --)\s+)(?:[A-Z][A-Z0-9_]*=\S*\s+)*(?:go test|gotestsum|node --test|node \S*(?:test|spec)\S*\.[cm]?[jt]s\b|pytest|python3? -m (?:pytest|unittest)|(?:npx |pnpm (?:exec |dlx )?)?(?:jest|vitest|mocha|playwright test)|(?:npm|pnpm|yarn|bun) (?:run )?test|cargo (?:test|nextest)|task (?:test|ci)\b|make (?:test|check)\b|mvn (?:test|verify)|\./gradlew test|gradle test|rspec|phpunit|deno test|swift test|dotnet test)`)
	buildCommand = regexp.MustCompile(`(?:^|[;&|(\n]\s*|\btime\s+|\brun\.sh\s+|\bmise (?:run|exec --)\s+)(?:[A-Z][A-Z0-9_]*=\S*\s+)*(?:go (?:build|vet)|golangci-lint|staticcheck|(?:npx |pnpm (?:exec )?)?(?:tsc|eslint|biome|prettier --check)|ruff|mypy|pyright|cargo (?:build|check|clippy)|(?:npm|pnpm|yarn|bun) (?:run )?(?:build|lint|typecheck|type-check)|task (?:lint|build)\b|make (?:build|lint)\b|shellcheck|dotnet build|swift build)`)
)

// The lines test runners print when tests fail, and when they all passed. A
// summary is trusted over the exit status: an agent pipes the output of a run
// through tail or grep, and the pipe reports the exit status of tail.
var (
	testFailed = regexp.MustCompile(`(?m)^--- FAIL:|^FAIL(?:\s|$)|^panic: |^=+ .*\b\d+ (?:failed|errors?)\b|^FAILED |^Tests?:?\s+.*\b\d+ failed|^\s*Test Files\s+\d+ failed|test result: FAILED|^\s+\d+ failing`)
	testPassed = regexp.MustCompile(`(?m)^ok\s+\S|^PASS$|^=+ .*\b\d+ passed\b|^Tests?:?\s+\d+ passed|^\s*Test Files\s+\d+ passed|test result: ok|^\s+\d+ passing|^OK(?: \(|$)`)
	// buildFailed are the error lines of compilers, type checkers and linters.
	buildFailed = regexp.MustCompile(`(?m)^\S+\.go:\d+:\d+: |error TS\d+|^error(?:\[E\d+\])?: |✖ \d+ problems?|^\d+ issues?:|^Found \d+ errors?|^\S+:\d+:\d+: (?:error|E\d+)|: error: `)
)

// ClassifyCommand returns the kind of check a shell command runs, or CheckNone.
// A command that runs both a build and the tests is a test run: the tests build
// the code first.
func ClassifyCommand(command string) CheckKind {
	switch {
	case testCommand.MatchString(command):
		return CheckTest
	case buildCommand.MatchString(command):
		return CheckBuild
	}
	return CheckNone
}

// JudgeCheck decides how a check ended. exitFailed is what the exit status
// said; output is what the command printed.
//
// A failure is believed from either the exit status or the output; a pass needs
// the output to say so, or an exit status that a pipe cannot have hidden. In 359
// test runs of real sessions, 87 printed failures and exited with 0: their
// output went through a pipe.
func JudgeCheck(kind CheckKind, command string, exitFailed bool, output string) Outcome {
	failed, passed := testFailed, testPassed
	if kind == CheckBuild {
		failed, passed = buildFailed, nil
	}
	switch {
	case exitFailed, failed.MatchString(output):
		return OutcomeFail
	case passed != nil && passed.MatchString(output):
		return OutcomePass
	case !piped(command) || kind == CheckBuild:
		// A build prints nothing when it succeeds; with no error line, it passed.
		return OutcomePass
	}
	return OutcomeUnknown
}

// piped reports whether a command sends its output through a pipe, whose exit
// status is that of its last command. "||" is not a pipe.
func piped(command string) bool {
	return strings.Contains(strings.ReplaceAll(command, "||", ""), "|")
}

// CheckRuns counts the runs of one kind of check.
type CheckRuns struct {
	Runs   int `json:"runs"`
	Passed int `json:"passed,omitzero"`
	Failed int `json:"failed,omitzero"`
	// Masked are the failures whose exit status said success.
	Masked int `json:"masked,omitzero"`
	// Last is the outcome of the latest run.
	Last Outcome `json:"last,omitzero"`
}

// countRun counts one run of a check.
func countRun(c *CheckRuns, o Outcome, masked bool) {
	c.Runs++
	c.Last = o
	switch o {
	case OutcomePass:
		c.Passed++
	case OutcomeFail:
		c.Failed++
		if masked {
			c.Masked++
		}
	case OutcomeUnknown:
	}
}

// FailRate returns the share of the decided runs that failed, in percent. ok
// is false before a run whose outcome is known.
func (c CheckRuns) FailRate() (float64, bool) {
	decided := c.Passed + c.Failed
	if decided == 0 {
		return 0, false
	}
	return float64(c.Failed) / float64(decided) * percent, true
}

// MaxUnverified bounds the files remembered as edited since the tests last
// passed.
const MaxUnverified = 200

// pendingKept bounds the calls waiting for their result. A result follows its
// call within a few entries; a call whose result never came (an interrupted
// session) must not be kept for ever.
const pendingKept = 32

// Quality is what the session's tool calls say about the quality of its work:
// the tests and builds it ran and how they ended, the edits that failed or went
// over a file again, the code changed since the tests last passed, and the calls
// repeated as they were.
type Quality struct {
	Tests  CheckRuns `json:"tests,omitzero"`
	Builds CheckRuns `json:"builds,omitzero"`
	// Edits are the calls that change a file; EditFailures those that failed
	// (the text to replace was not found, or found twice); ReEdits those of a
	// file the session had edited before.
	Edits        int `json:"edits,omitzero"`
	EditFailures int `json:"edit_failures,omitzero"`
	ReEdits      int `json:"re_edits,omitzero"`
	// EditFailStreak are the edits that failed in a row, up to now. After one
	// failed edit, the chance that an agent's edit eventually succeeds fell from
	// 90.5% to 57.2% (Yang et al., "SWE-agent", NeurIPS 2024).
	EditFailStreak int `json:"edit_fail_streak,omitzero"`
	// RedCalls are the tool calls made since the tests began to fail: failed
	// trajectories mostly went on without progress after the failure was
	// certain (Zhao et al., "Failure as a Process", 2026).
	RedCalls int `json:"red_calls,omitzero"`
	// Claims are the texts that said the tests passed while they had failed,
	// had not run or had not run since an edit, or that said the work was done
	// while the tests failed. 26% of failed trajectories reported a success
	// (Zhao et al. 2026); inaccurate self-reports were 22.58% of the developers'
	// complaints in 20,574 sessions (Tang et al., "How Coding Agents Fail Their
	// Users", 2026).
	Claims int `json:"claims,omitzero"`
	// EditsSinceRun are the source edits since the tests last ran.
	EditsSinceRun int `json:"edits_since_run,omitzero"`
	// RedSince is when the tests began to fail, zero while they pass or before
	// they ran.
	RedSince time.Time `json:"red_since,omitzero"`
	// Repeats are the tool calls identical to the call just before: the same
	// tool with the same input.
	Repeats int `json:"repeats,omitzero"`
	// Unverified are the source files edited since the tests last passed.
	Unverified []string `json:"unverified,omitzero"`
	// Pending are the calls whose result has not been read yet.
	Pending []PendingCall `json:"pending,omitzero"`
	// LastCall is the signature of the latest tool call.
	LastCall string `json:"last_call,omitzero"`
}

// PendingCall is a tool call whose result decides something: a check, or an
// edit of a file.
type PendingCall struct {
	ID      string    `json:"id"`
	Check   CheckKind `json:"check,omitzero"`
	Command string    `json:"command,omitzero"`
	File    string    `json:"file,omitzero"`
}

// Call records a tool call by its signature, counting it as a repeat when it is
// the call just before again.
func (q *Quality) Call(signature string) {
	if signature != "" && signature == q.LastCall {
		q.Repeats++
	}
	q.LastCall = signature
}

// Await keeps a call until its result is read.
func (q *Quality) Await(p PendingCall) {
	q.Pending = append(q.Pending, p)
	if n := len(q.Pending); n > pendingKept {
		q.Pending = slices.Clone(q.Pending[n-pendingKept:])
	}
}

// Resolve returns the call a result answers, and forgets it.
func (q *Quality) Resolve(id string) (PendingCall, bool) {
	i := slices.IndexFunc(q.Pending, func(p PendingCall) bool { return p.ID == id })
	if i < 0 {
		return PendingCall{}, false
	}
	p := q.Pending[i]
	q.Pending = slices.Delete(q.Pending, i, i+1)
	return p, true
}

// EditFailed records an edit that failed.
func (q *Quality) EditFailed() {
	q.EditFailures++
	q.EditFailStreak++
}

// Edited records an edit that succeeded: a source file is unverified until the
// tests pass again.
func (q *Quality) Edited(file string) {
	q.EditFailStreak = 0
	if IsSourcePath(file) {
		q.EditsSinceRun++
	}
	if !IsSourcePath(file) || len(q.Unverified) >= MaxUnverified || slices.Contains(q.Unverified, file) {
		return
	}
	q.Unverified = append(q.Unverified, file)
}

// Checked records how a check ended at a time. Passing tests verify every edit
// before them; failing tests stay red from the first failure until they pass.
func (q *Quality) Checked(kind CheckKind, o Outcome, masked bool, at time.Time) {
	switch kind {
	case CheckTest:
		q.EditsSinceRun = 0
		countRun(&q.Tests, o, masked)
		switch o {
		case OutcomePass:
			q.Unverified = nil
			q.RedSince = time.Time{}
			q.RedCalls = 0
		case OutcomeFail:
			if q.RedSince.IsZero() {
				q.RedSince = at
			}
		case OutcomeUnknown:
		}
	case CheckBuild:
		countRun(&q.Builds, o, masked)
	case CheckNone:
	}
}

// sourceExtensions are the extensions of files that hold code. Documents,
// data and configuration are left out: tests do not verify them.
var sourceExtensions = []string{
	".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".java", ".kt", ".kts", ".scala",
	".rs", ".rb", ".php", ".cs", ".fs", ".c", ".cc", ".cpp", ".h", ".hpp", ".m", ".swift", ".dart",
	".sh", ".bash", ".zsh", ".sql", ".vue", ".svelte", ".lua", ".ex", ".exs", ".erl", ".hs", ".clj",
}

// IsSourcePath reports whether a file holds code.
func IsSourcePath(file string) bool {
	return slices.Contains(sourceExtensions, strings.ToLower(path.Ext(file)))
}

// testPath matches the files that hold tests, by the conventions of the common
// languages: foo_test.go, test_foo.py, foo.test.ts, foo.spec.js, FooTest.java,
// and the files under test, tests, spec and __tests__ directories.
var testPath = regexp.MustCompile(`(?:^|/)(?i:tests?|specs?|__tests__|e2e)/|_test\.\w+$|(?:^|/)test_[^/]*\.py$|\.(?:test|spec)\.\w+$|[a-z0-9](?:Test|Tests|Spec)\.(?:java|kt|scala|cs|swift|php)$`)

// IsTestPath reports whether a file holds tests.
func IsTestPath(file string) bool { return testPath.MatchString(file) }

// FileChange is a file changed against HEAD and its lines.
type FileChange struct {
	Path    string
	Added   int
	Deleted int
}

// Lines returns the lines the file changes.
func (f FileChange) Lines() int { return f.Added + f.Deleted }

// Spread is how widely a change is spread: the files, directories and
// top-level directories (subsystems) it touches, and the entropy of its lines
// over the files, normalised to 0..1 (Hassan, "Predicting Faults Using the
// Complexity of Code Changes", ICSE 2009). The four are the diffusion measures
// of just-in-time defect prediction (Kamei et al., TSE 2013): the wider a change,
// the likelier it brings a defect.
type Spread struct {
	Files, Dirs, Subsystems int
	Entropy                 float64
}

// ChangeSpread measures the spread of the source files of a change. ok is false
// when the change touches no source file.
func ChangeSpread(changes []FileChange) (Spread, bool) {
	var lines []float64
	dirs, roots := map[string]bool{}, map[string]bool{}
	total := 0.0
	for _, c := range changes {
		if !IsSourcePath(c.Path) {
			continue
		}
		dirs[path.Dir(c.Path)] = true
		root, _, _ := strings.Cut(c.Path, "/")
		roots[root] = true
		lines = append(lines, float64(c.Lines()))
		total += float64(c.Lines())
	}
	if len(lines) == 0 {
		return Spread{}, false
	}
	s := Spread{Files: len(lines), Dirs: len(dirs), Subsystems: len(roots)}
	if len(lines) > 1 && total > 0 {
		for _, n := range lines {
			if p := n / total; p > 0 {
				s.Entropy -= p * math.Log2(p)
			}
		}
		s.Entropy /= math.Log2(float64(len(lines)))
	}
	return s, true
}

// TestShare returns the share of the changed source lines that are in test
// files, in percent. ok is false when no source line changed.
func TestShare(changes []FileChange) (float64, bool) {
	var all, tests int
	for _, c := range changes {
		if !IsSourcePath(c.Path) {
			continue
		}
		all += c.Lines()
		if IsTestPath(c.Path) {
			tests += c.Lines()
		}
	}
	if all == 0 {
		return 0, false
	}
	return float64(tests) / float64(all) * percent, true
}

// debtMark matches the markers of self-admitted technical debt in a line.
var debtMark = regexp.MustCompile(`\b(?:TODO|FIXME|HACK|XXX)\b`)

// MarksDebt reports whether a line admits a debt.
func MarksDebt(line string) bool { return debtMark.MatchString(line) }

// fixSubject matches the subject of a commit that fixes or reverts.
var fixSubject = regexp.MustCompile(`(?i)^(?:(?:fix|hotfix|bugfix|revert)(?:\([^)]*\))?!?:|Revert ")`)

// IsFixCommit reports whether a commit's subject says it fixes or reverts.
func IsFixCommit(subject string) bool { return fixSubject.MatchString(subject) }

// mockMark matches a line of a test that makes a test double: a mock, a stub
// or a patch, in the frameworks of the common languages. Coding agents' commits
// added mocks more often than people's: 36% against 26% (Hora and Robbes, "Are
// Coding Agents Generating Over-Mocked Tests?", MSR 2026).
var mockMark = regexp.MustCompile(`(?i)\bmock|\bstub\b|\bspy(?:On)?\(|jest\.fn|vi\.fn|sinon\.|monkeypatch|@patch\b|\bpatch\(|\bfake[A-Z_]`)

// MarksMock reports whether a line makes a test double.
func MarksMock(line string) bool { return mockMark.MatchString(line) }

// dependencyLine matches a line that declares a dependency, by manifest. Lock
// files are left out: they list what the manifests already declared.
var dependencyLine = map[string]*regexp.Regexp{
	"go.mod":           regexp.MustCompile(`^\s*(?:require\s+)?[a-z0-9.\-]+\.[a-z]{2,}/\S+\s+v\d`),
	"package.json":     regexp.MustCompile(`^\s*"(?:@[^"/]+/)?[^"@/]+"\s*:\s*"(?:[\^~<>=]*\d|workspace:|catalog:|npm:|github:|git\+|file:|link:|latest)`),
	"requirements.txt": regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-\[\]]*\s*(?:[=<>!~]=?.*)?$`),
	"pyproject.toml":   regexp.MustCompile(`^\s*"[A-Za-z0-9][A-Za-z0-9_.\-\[\]]*\s*(?:[=<>!~]=?[^"]*)?",?\s*$`),
	"Cargo.toml":       regexp.MustCompile(`^[A-Za-z0-9_\-]+\s*=\s*(?:"[\^~=<>]*\d|\{.*\b(?:version|git|path)\s*=)`),
	"Gemfile":          regexp.MustCompile(`^\s*gem\s+['"]`),
}

// notDependencies are the keys of a manifest whose value looks like a version
// but is no dependency: the package's own version and the toolchain's.
var notDependencies = regexp.MustCompile(`^\s*"?(?:version|node|npm|pnpm|yarn|edition|rust-version|packageManager)"?\s*[:=]`)

// AddsDependency reports whether an added line of a file declares a
// dependency. Agents add dependencies that do not exist: 19.6% of the packages
// that sixteen models generated were hallucinated (Spracklen et al., "We Have a
// Package for You!", USENIX Security 2025).
func AddsDependency(file, line string) bool {
	name := path.Base(file)
	if strings.HasPrefix(name, "requirements") && strings.HasSuffix(name, ".txt") {
		name = "requirements.txt"
	}
	pattern, ok := dependencyLine[name]
	return ok && !notDependencies.MatchString(line) && pattern.MatchString(line)
}

// aiCoauthor matches the co-author trailer of a coding agent or an assistant.
var aiCoauthor = regexp.MustCompile(`(?i)claude|anthropic|copilot|codex|openai|cursor|devin|gemini|aider|windsurf`)

// IsAICoauthor reports whether a commit's co-authors name an AI. Claude Code
// signs every commit it makes with such a trailer; a commit whose trailer was
// removed is not counted, so the share is a lower bound (Robbes et al.,
// "Agentic Much? Adoption of Coding Agents on GitHub", 2026).
func IsAICoauthor(coauthors string) bool { return aiCoauthor.MatchString(coauthors) }

// Claimed records a text of the agent, counting it when it claims more than
// the checks showed.
func (q *Quality) Claimed(text string) {
	// Before a test run was recognised, nothing tells a claim from the truth:
	// the tests may have run in a way the status line does not know. A run
	// whose outcome is unknown is no ground either: an empty output piped
	// through grep -v ok means that everything passed.
	unverified := q.Tests.Runs > 0 && (q.Tests.Last == OutcomeFail || q.EditsSinceRun > 0)
	if ClaimsTests(text) && unverified || ClaimsDone(text) && q.Tests.Last == OutcomeFail {
		q.Claims++
	}
}
