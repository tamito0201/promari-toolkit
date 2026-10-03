package model

import (
	"regexp"
	"slices"
)

const (
	// maxSeenKept bounds the reads and searches remembered since the last edit.
	maxSeenKept = 200
	// maxTurnTokensKept bounds the token totals of the prompts kept.
	maxTurnTokensKept = 200
	// bytesPerToken converts the bytes of a tool's output to tokens: about four
	// characters of English text make a token. Japanese takes more bytes per
	// character and fewer per token, so the estimate is rough either way.
	bytesPerToken = 4.0
	// LongEditRun is the run of edits without a read from which an agent's
	// trajectories mostly failed: about 80% with five or more edits in a row,
	// 59% without (Oderinwale, "Agent trajectories as programs", 2026).
	LongEditRun = 5
)

// Trace is how the agent works through a session: how it explores and edits,
// what it reads again, how much of the context its tools' output takes, and how
// unevenly the prompts cost.
type Trace struct {
	// Explores are the calls that read or search (Read, Grep, Glob, LS).
	Explores int `json:"explores,omitzero"`
	// Rereads and Researches are reads and searches identical to one made
	// since the last edit, whose answer the context holds already. In Claude
	// Code, such reads came up in 64-92% of the tasks and took 5-11% of their
	// cost (Hu et al., "Analyzing and Mitigating Cost-Inefficient Behaviors in
	// Coding Agents", 2026).
	Rereads    int `json:"rereads,omitzero"`
	Researches int `json:"researches,omitzero"`
	// Seen are the signatures of the reads and searches since the last edit.
	Seen []string `json:"seen,omitzero"`
	// EditRun is the current run of edits with no other call in between, and
	// MaxEditRun the longest. A shell command counts as a look: in Claude Code
	// the agent reads with cat, grep and git as often as with Read.
	EditRun    int `json:"edit_run,omitzero"`
	MaxEditRun int `json:"max_edit_run,omitzero"`
	// ObservedBytes are the bytes of the tools' output since the last
	// compaction, and LargestObserved the largest single output.
	ObservedBytes   float64 `json:"observed_bytes,omitzero"`
	LargestObserved float64 `json:"largest_observed,omitzero"`
	// PromptsSinceCompact are the prompts since the last compaction, or since
	// the start.
	PromptsSinceCompact int `json:"prompts_since_compact,omitzero"`
	// TurnTokens are the tokens of each prompt that ended, oldest first, and
	// TurnMark the session's tokens when the current prompt began.
	TurnTokens []float64 `json:"turn_tokens,omitzero"`
	TurnMark   float64   `json:"turn_mark,omitzero"`
	// Prompted is true once a prompt began.
	Prompted bool `json:"prompted,omitzero"`
}

// The kinds of calls a trace tells apart.
const (
	CallRead = iota + 1
	CallSearch
	CallEdit
	CallOther
)

// Called records a call of a kind with its signature.
func (t *Trace) Called(kind int, signature string) {
	switch kind {
	case CallRead, CallSearch:
		t.Explores++
		t.EditRun = 0
		if signature != "" && slices.Contains(t.Seen, signature) {
			if kind == CallRead {
				t.Rereads++
			} else {
				t.Researches++
			}
			return
		}
		if signature != "" && len(t.Seen) < maxSeenKept {
			t.Seen = append(t.Seen, signature)
		}
	case CallEdit:
		// An edit changes what a read would answer.
		t.Seen = nil
		t.EditRun++
		t.MaxEditRun = max(t.MaxEditRun, t.EditRun)
	default:
		t.EditRun = 0
	}
}

// Observed records the output of a tool.
func (t *Trace) Observed(bytes int) {
	t.ObservedBytes += float64(bytes)
	t.LargestObserved = max(t.LargestObserved, float64(bytes))
}

// ObservedTokens estimates the tokens of the tools' output since the last
// compaction, and of the largest one.
func (t *Trace) ObservedTokens() (all, largest float64) {
	return t.ObservedBytes / bytesPerToken, t.LargestObserved / bytesPerToken
}

// Compacted starts the counts that run from one compaction to the next.
func (t *Trace) Compacted() {
	t.ObservedBytes, t.LargestObserved, t.PromptsSinceCompact = 0, 0, 0
}

// Prompt records a prompt at the session's token total: the prompt before it
// ends with it.
func (t *Trace) Prompt(tokens float64) {
	// A prompt that cost nothing (a command Claude Code answered itself, a
	// prompt queued behind another) is no turn of the agent's.
	if spent := tokens - t.TurnMark; t.Prompted && spent > 0 {
		t.TurnTokens = append(t.TurnTokens, spent)
		if n := len(t.TurnTokens); n > maxTurnTokensKept {
			t.TurnTokens = slices.Clone(t.TurnTokens[n-maxTurnTokensKept:])
		}
	}
	t.Prompted = true
	t.TurnMark = tokens
	t.PromptsSinceCompact++
}

// TurnSpread returns the median and the largest tokens of the prompts that
// ended. ok is false before two of them: one prompt has no spread.
func (t *Trace) TurnSpread() (mid, largest float64, ok bool) {
	if len(t.TurnTokens) < 2 {
		return 0, 0, false
	}
	sorted := slices.Sorted(slices.Values(t.TurnTokens))
	return quantile(sorted, median), sorted[len(sorted)-1], true
}

// ExploreRatio returns the reads and searches per edit. ok is false before an
// edit.
func (t *Trace) ExploreRatio(edits int) (float64, bool) {
	if edits == 0 {
		return 0, false
	}
	return float64(t.Explores) / float64(edits), true
}

// testClaim matches a claim that tests or checks passed, and doneClaim a claim
// that the work is done, in English and Japanese.
var (
	testClaim = regexp.MustCompile(`(?i)\b(?:all )?tests? (?:now )?(?:pass(?:ed|es|ing)?|(?:are|is) (?:green|passing))\b|テスト.{0,12}(?:通り|通っ|通過|成功|パス)`)
	doneClaim = regexp.MustCompile(`(?i)\b(?:I(?:'ve| have)? fixed|(?:is|are|now) fixed|fixed (?:it|this|that)\b|works now)\b|(?:修正|直|解消|解決)(?:しました|済み)|動作(?:を)?確認(?:しました|済み)`)
)

// ClaimsTests reports whether a text says tests or checks passed.
func ClaimsTests(text string) bool { return testClaim.MatchString(text) }

// ClaimsDone reports whether a text says a problem is fixed.
func ClaimsDone(text string) bool { return doneClaim.MatchString(text) }

// skipMark matches a line that skips or expects the failure of a test, and
// assertMark a line that asserts.
var (
	skipMark   = regexp.MustCompile(`\bt\.Skip(?:f|Now)?\(|@(?:pytest\.mark\.)?(?:skip|xfail)\b|pytest\.(?:skip|xfail)\(|\b(?:it|test|describe)\.(?:skip|todo)\(|\bx(?:it|describe|test)\(|@(?:Disabled|Ignore)\b|#\[ignore\]|unittest\.skip`)
	assertMark = regexp.MustCompile(`\bassert|\bexpect\(|\bt\.(?:Error|Errorf|Fatal|Fatalf|Fail|FailNow)\(|\brequire\.|\bshould\b|\.toBe|\.toEqual`)
)

// MarksSkip reports whether a line of a test skips it.
func MarksSkip(line string) bool { return skipMark.MatchString(line) }

// MarksAssert reports whether a line of a test asserts.
func MarksAssert(line string) bool { return assertMark.MatchString(line) }
