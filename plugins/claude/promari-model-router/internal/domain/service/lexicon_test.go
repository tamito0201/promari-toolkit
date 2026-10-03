package service_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
)

// testClassifier is a compact classifier configuration the cases below reason about.
var testClassifier = model.ClassifierSettings{
	StrongWeight: 3, WeakWeight: 1, ClassCap: 9, MinConfidence: 3, MinMargin: 2, ConfidentMultiple: 2,
	ContinuationMaxChars: 8, CodeFenceBonus: 1, StackTraceBonus: 3, LongPromptChars: 200, LongPromptLines: 5,
	LongPromptBonus: 1, MaxReasonHits: 1, MaxDangerHits: 1,
}

// testLexicon exercises every rule of Compile: "find" appears as a strong
// lookup cue and a weak standard cue (the weaker duplicate loses), "一覧" as a
// weak lookup cue and a strong mechanical cue (the stronger one replaces it),
// and an empty phrase is ignored.
func testLexicon() *service.Lexicon {
	return (&service.Lexicon{
		Classes: map[model.Class]service.PhraseSet{
			model.ClassLookup:       {Strong: []string{"探して", "find"}, Weak: []string{"一覧"}},
			model.ClassMechanical:   {Strong: []string{"一覧"}},
			model.ClassStandard:     {Strong: []string{"テストを書いて"}, Weak: []string{"直して", "find"}},
			model.ClassComplex:      {Strong: []string{"原因"}, Weak: []string{""}},
			model.ClassArchitecture: {Strong: []string{"見直して"}},
		},
		Danger:       []string{"本番", "password"},
		Codex:        map[string][]string{"review": {"レビュー"}, "task": {"一括"}},
		Continuation: []string{"続けて"},
		Context:      []string{"上記"},
		Correction:   []string{"違う"},
	}).Compile(testClassifier)
}

func TestExtractSignals(t *testing.T) {
	lex := testLexicon()
	scores := func(kv ...any) map[model.Class]int {
		out := map[model.Class]int{}
		for i := 0; i < len(kv); i += 2 {
			out[kv[i].(model.Class)] = kv[i+1].(int)
		}
		return out
	}
	capped := (&service.Lexicon{
		Classes: map[model.Class]service.PhraseSet{model.ClassComplex: {Strong: []string{"原因"}}},
	}).Compile(model.ClassifierSettings{StrongWeight: 3, ClassCap: 4, StackTraceBonus: 3, LongPromptChars: 99, LongPromptLines: 99})
	cappedTwo := (&service.Lexicon{
		Classes: map[model.Class]service.PhraseSet{model.ClassComplex: {Strong: []string{"原因", "競合"}}},
	}).Compile(model.ClassifierSettings{StrongWeight: 3, ClassCap: 4, LongPromptChars: 99, LongPromptLines: 99})
	// Phrases of equal length that overlap: which one masks the other is
	// decided by weight, then by class order ("ab"/"bc" in "abc", "xy"/"yz"
	// in "xyz"); "same" is listed with equal weight in two classes.
	overlap := (&service.Lexicon{
		Classes: map[model.Class]service.PhraseSet{
			model.ClassLookup:   {Strong: []string{"ab", "xy", "same"}},
			model.ClassStandard: {Strong: []string{"yz", "same"}, Weak: []string{"bc"}},
		},
	}).Compile(testClassifier)
	tests := []struct {
		name string
		lex  *service.Lexicon
		text string
		want service.Signals
	}{
		{
			name: "scores are capped per class",
			lex:  capped,
			text: "原因\npanic: x",
			want: service.Signals{
				Normalised: "原因\npanic: x", Chars: 11, Lang: model.LangMixed, ClassCap: 4,
				Scores:     scores(model.ClassComplex, 4),
				StrongHits: map[model.Class][]string{model.ClassComplex: {"原因"}},
				StackTrace: true,
			},
		},
		{
			name: "lexicon hits alone are capped per class",
			lex:  cappedTwo,
			text: "原因と競合",
			want: service.Signals{
				Normalised: "原因と競合", Chars: 5, Lang: model.LangJA, ClassCap: 4,
				Scores:     scores(model.ClassComplex, 4),
				StrongHits: map[model.Class][]string{model.ClassComplex: {"原因", "競合"}},
			},
		},
		{
			name: "an equal-weight duplicate keeps the first class in class order",
			lex:  overlap,
			text: "same",
			want: service.Signals{
				Normalised: "same", Chars: 4, Lang: model.LangEN, ClassCap: 9,
				Scores: scores(model.ClassLookup, 3), StrongHits: map[model.Class][]string{model.ClassLookup: {"same"}},
			},
		},
		{
			name: "on an equal length the stronger phrase masks the weaker one",
			lex:  overlap,
			text: "abc",
			want: service.Signals{
				Normalised: "abc", Chars: 3, Lang: model.LangEN, ClassCap: 9,
				Scores: scores(model.ClassLookup, 3), StrongHits: map[model.Class][]string{model.ClassLookup: {"ab"}},
			},
		},
		{
			name: "on an equal length and weight the class first in class order masks",
			lex:  overlap,
			text: "xyz",
			want: service.Signals{
				Normalised: "xyz", Chars: 3, Lang: model.LangEN, ClassCap: 9,
				Scores: scores(model.ClassLookup, 3), StrongHits: map[model.Class][]string{model.ClassLookup: {"xy"}},
			},
		},
		{
			name: "a weak cue scores but is not a strong hit",
			text: "直して",
			want: service.Signals{
				Normalised: "直して", Chars: 3, Lang: model.LangJA, ClassCap: 9,
				Scores: scores(model.ClassStandard, 1), StrongHits: map[model.Class][]string{},
			},
		},
		{
			name: "codex review wins over a codex task",
			text: "一括でレビュー",
			want: service.Signals{
				Normalised: "一括でレビュー", Chars: 7, Lang: model.LangJA, ClassCap: 9,
				Scores: scores(), StrongHits: map[model.Class][]string{}, Codex: model.CodexReview,
			},
		},
		{
			name: "a continuation exactly at the length limit",
			text: "続けてくださいね",
			want: service.Signals{
				Normalised: "続けてくださいね", Chars: 8, Lang: model.LangJA, ClassCap: 9,
				Scores: scores(), StrongHits: map[model.Class][]string{}, Continuation: true,
			},
		},
		{
			name: "exactly long_prompt_chars is a long prompt",
			text: strings.Repeat("x", 200),
			want: service.Signals{
				Normalised: strings.Repeat("x", 200), Chars: 200, Lang: model.LangEN, ClassCap: 9,
				Scores: scores(model.ClassArchitecture, 1, model.ClassComplex, 1), StrongHits: map[model.Class][]string{},
				LongPrompt: true,
			},
		},
		{
			name: "blank text",
			text: "  \n ",
			want: service.Signals{Lang: model.LangNone, ClassCap: 9},
		},
		{
			name: "a strong japanese cue without spaces",
			text: "ファイルを探して",
			want: service.Signals{
				Normalised: "ファイルを探して", Chars: 8, Lang: model.LangJA, ClassCap: 9,
				Scores: scores(model.ClassLookup, 3), StrongHits: map[model.Class][]string{model.ClassLookup: {"探して"}},
			},
		},
		{
			name: "the longest match masks the shorter phrase",
			text: "設計を見直して",
			want: service.Signals{
				Normalised: "設計を見直して", Chars: 7, Lang: model.LangJA, ClassCap: 9,
				Scores: scores(model.ClassArchitecture, 3), StrongHits: map[model.Class][]string{model.ClassArchitecture: {"見直して"}},
			},
		},
		{
			name: "duplicates keep the stronger class and full width is normalised",
			text: "ＦＩＮＤ 一覧",
			want: service.Signals{
				Normalised: "find 一覧", Chars: 7, Lang: model.LangMixed, ClassCap: 9,
				Scores: scores(model.ClassLookup, 3, model.ClassMechanical, 3),
				StrongHits: map[model.Class][]string{
					model.ClassLookup: {"find"}, model.ClassMechanical: {"一覧"},
				},
			},
		},
		{
			name: "danger, codex review, context and correction",
			text: "上記の本番コードをレビューして、違う",
			want: service.Signals{
				Normalised: "上記の本番コードをレビューして、違う", Chars: 18, Lang: model.LangJA, ClassCap: 9,
				Scores: scores(), StrongHits: map[model.Class][]string{},
				Danger: []string{"本番"}, Codex: model.CodexReview, ContextCues: []string{"上記"}, Correction: true,
			},
		},
		{
			name: "codex task",
			text: "一括で置換",
			want: service.Signals{
				Normalised: "一括で置換", Chars: 5, Lang: model.LangJA, ClassCap: 9,
				Scores: scores(), StrongHits: map[model.Class][]string{}, Codex: model.CodexTask,
			},
		},
		{
			name: "a short continuation",
			text: "続けて",
			want: service.Signals{
				Normalised: "続けて", Chars: 3, Lang: model.LangJA, ClassCap: 9,
				Scores: scores(), StrongHits: map[model.Class][]string{}, Continuation: true,
			},
		},
		{
			name: "a long continuation is a new task",
			text: "続けて、残りも全部やる",
			want: service.Signals{
				Normalised: "続けて、残りも全部やる", Chars: 11, Lang: model.LangJA, ClassCap: 9,
				Scores: scores(), StrongHits: map[model.Class][]string{},
			},
		},
		{
			name: "a code fence and a stack trace",
			text: "```\npanic: boom\n```",
			want: service.Signals{
				Normalised: "```\npanic: boom\n```", Chars: 19, Lang: model.LangEN, ClassCap: 9,
				Scores: scores(model.ClassStandard, 1, model.ClassComplex, 3), StrongHits: map[model.Class][]string{},
				CodeFence: true, StackTrace: true,
			},
		},
		{
			name: "a long prompt by lines",
			text: "a\nb\nc\nd\ne",
			want: service.Signals{
				Normalised: "a\nb\nc\nd\ne", Chars: 9, Lang: model.LangEN, ClassCap: 9,
				Scores: scores(model.ClassArchitecture, 1, model.ClassComplex, 1), StrongHits: map[model.Class][]string{},
				LongPrompt: true,
			},
		},
		{
			name: "bonuses add up per class",
			text: "原因\nKeyError: x\n" + strings.Repeat("x", 200),
			want: service.Signals{
				Normalised: "原因\nkeyerror: x\n" + strings.Repeat("x", 200), Chars: 215, Lang: model.LangEN, ClassCap: 9,
				Scores:     scores(model.ClassComplex, 7, model.ClassArchitecture, 1),
				StrongHits: map[model.Class][]string{model.ClassComplex: {"原因"}},
				StackTrace: true, LongPrompt: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lex
			if tt.lex != nil {
				l = tt.lex
			}
			if diff := cmp.Diff(tt.want, l.ExtractSignals(tt.text), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("ExtractSignals() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestExtractSignalsShippedLexicon pins behaviour of the shipped lexicon that
// users rely on.
func TestExtractSignalsShippedLexicon(t *testing.T) {
	lex, _, _ := fixtures(t)
	type view struct {
		Complex, Standard int
		Lang              model.Language
		Danger            []string
		HasUserService    bool
		StackTrace        bool
		Continuation      bool
	}
	project := func(s service.Signals) view {
		return view{
			Complex: s.Scores[model.ClassComplex], Standard: s.Scores[model.ClassStandard], Lang: s.Lang, Danger: s.Danger,
			HasUserService: strings.Contains(s.Normalised, "userservice"), StackTrace: s.StackTrace, Continuation: s.Continuation,
		}
	}
	tests := []struct {
		name  string
		text  string
		check func(view) view // keeps only the fields the case is about
		want  view
	}{
		{
			name: "japanese without spaces is scored", text: "CIでたまに落ちるテストの原因を調べて",
			check: func(v view) view { return view{Complex: min(v.Complex, 3), Lang: v.Lang} },
			want:  view{Complex: 3, Lang: model.LangJA},
		},
		{
			name: "longest match masks the shorter phrase", text: "この設計書のアーキテクチャを見直して",
			check: func(v view) view { return view{Standard: v.Standard} },
			want:  view{Standard: 0},
		},
		{
			name: "author is not auth", text: "list all files written by the author",
			check: func(v view) view { return view{Danger: v.Danger} },
			want:  view{},
		},
		{
			name: "full-width text is normalised", text: "ＵｓｅｒＳｅｒｖｉｃｅ を探して",
			check: func(v view) view { return view{HasUserService: v.HasUserService} },
			want:  view{HasUserService: true},
		},
		{
			name: "stack trace raises complex", text: "直して\nTraceback (most recent call last):\n  File \"a.py\"\nKeyError: 'x'",
			check: func(v view) view { return view{StackTrace: v.StackTrace} },
			want:  view{StackTrace: true},
		},
		{
			name: "continuation", text: "続けて",
			check: func(v view) view { return view{Continuation: v.Continuation} },
			want:  view{Continuation: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.check(project(lex.ExtractSignals(tt.text))), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("ExtractSignals() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRuleClassify(t *testing.T) {
	scores := map[model.Class]int{}
	tests := []struct {
		name string
		sig  service.Signals
		want model.Classification
	}{
		{
			name: "empty prompt",
			sig:  service.Signals{Scores: scores},
			want: model.Classification{Scores: scores, Reasons: []string{"empty"}},
		},
		{
			name: "below the confidence threshold",
			sig:  service.Signals{Chars: 5, Scores: map[model.Class]int{model.ClassLookup: 2}},
			want: model.Classification{Chars: 5, Scores: map[model.Class]int{model.ClassLookup: 2}, Confidence: 2, Margin: 2, Reasons: []string{"below-threshold"}},
		},
		{
			name: "a thin margin is ambiguous",
			sig:  service.Signals{Chars: 5, Scores: map[model.Class]int{model.ClassLookup: 3, model.ClassStandard: 2}},
			want: model.Classification{
				Chars: 5, Scores: map[model.Class]int{model.ClassLookup: 3, model.ClassStandard: 2},
				Confidence: 3, Margin: 1, Reasons: []string{"ambiguous:lookup/standard"},
			},
		},
		{
			// Confidence equals min_confidence and the margin equals min_margin:
			// both limits are inclusive.
			name: "a margin and a confidence exactly at their limits decide",
			sig:  service.Signals{Chars: 5, Scores: map[model.Class]int{model.ClassLookup: 3, model.ClassStandard: 1}},
			want: model.Classification{
				Class: model.ClassLookup, Chars: 5, Scores: map[model.Class]int{model.ClassLookup: 3, model.ClassStandard: 1},
				Confidence: 3, Margin: 2,
			},
		},
		{
			name: "a tie ranks the more demanding class first",
			sig:  service.Signals{Chars: 5, Scores: map[model.Class]int{model.ClassLookup: 3, model.ClassStandard: 3}},
			want: model.Classification{
				Chars: 5, Scores: map[model.Class]int{model.ClassLookup: 3, model.ClassStandard: 3},
				Confidence: 3, Reasons: []string{"ambiguous:standard/lookup"},
			},
		},
		{
			name: "a confident top class wins despite a thin margin",
			sig:  service.Signals{Chars: 5, Scores: map[model.Class]int{model.ClassLookup: 6, model.ClassStandard: 5}},
			want: model.Classification{
				Class: model.ClassLookup, Chars: 5, Scores: map[model.Class]int{model.ClassLookup: 6, model.ClassStandard: 5},
				Confidence: 6, Margin: 1,
			},
		},
		{
			name: "every reason is listed and hits are capped",
			sig: service.Signals{
				Chars: 50, Lang: model.LangJA, Codex: model.CodexTask, Continuation: true,
				Scores:      map[model.Class]int{model.ClassComplex: 6},
				StrongHits:  map[model.Class][]string{model.ClassComplex: {"原因", "競合"}},
				Danger:      []string{"本番", "password"},
				ContextCues: []string{"上記"}, StackTrace: true, LongPrompt: true,
			},
			want: model.Classification{
				Class: model.ClassComplex, Confidence: 6, Margin: 6, Scores: map[model.Class]int{model.ClassComplex: 6},
				Danger: true, Codex: model.CodexTask, Continuation: true, Lang: model.LangJA, Chars: 50,
				Reasons: []string{"danger:本番", "continuation", "complex:原因", "context-dependent", "stack-trace", "long-prompt"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.RuleClassify(tt.sig, testClassifier), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("RuleClassify() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLanguage(t *testing.T) {
	tests := []struct {
		name string
		text string
		want model.Language
	}{
		{name: "empty", text: "", want: model.LangNone},
		{name: "digits and symbols only", text: "123 !?", want: model.LangNone},
		{name: "japanese", text: "探してください", want: model.LangJA},
		{name: "english", text: "find it", want: model.LangEN},
		{name: "balanced mix", text: "find ファイル", want: model.LangMixed},
		{name: "one kana in four latin letters is still mixed", text: "あabcd", want: model.LangMixed},
		{name: "one kana in five latin letters is english", text: "あabcde", want: model.LangEN},
		{name: "japanese with a little latin", text: "UIがどこで定義されているかを確認して一覧にしてください", want: model.LangJA},
		{name: "english with a little japanese", text: "please find the definition of 型", want: model.LangEN},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.Language(tt.text)); diff != "" {
				t.Errorf("Language(%q) mismatch (-want +got):\n%s", tt.text, diff)
			}
		})
	}
}

func TestHits(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		phrases []string
		want    []string
	}{
		{name: "no phrases", text: "abc", want: nil},
		{name: "sorted and deduplicated", text: "prod password prod", phrases: []string{"prod", "password", "PROD"}, want: []string{"password", "prod"}},
		{name: "phrases are normalised", text: "userservice", phrases: []string{"ＵｓｅｒＳｅｒｖｉｃｅ"}, want: []string{"userservice"}},
		{name: "empty phrases never match", text: "abc", phrases: []string{"", "  "}, want: nil},
		{name: "no match", text: "abc", phrases: []string{"xyz"}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.Hits(tt.text, tt.phrases), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Hits() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNormalise(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "full width to half width", text: "ＡＢＣ１２３", want: "abc123"},
		{name: "half-width katakana", text: "ﾃｽﾄ", want: "テスト"},
		{name: "already normal", text: "abc", want: "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.Normalise(tt.text)); diff != "" {
				t.Errorf("Normalise() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
