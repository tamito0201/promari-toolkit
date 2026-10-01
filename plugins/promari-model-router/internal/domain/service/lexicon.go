// Package service holds the domain services of the router. Every function here
// is pure: same input, same output, no I/O. Data (lexicon, tiers, the learned
// artifact) comes in as arguments.
package service

import (
	"cmp"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/pkg/fp"
)

// PhraseSet is the strong and weak cue phrases of one class.
type PhraseSet struct {
	Strong []string `toml:"strong"`
	Weak   []string `toml:"weak"`
}

// Lexicon holds the bilingual cue phrases (data/lexicon.toml plus extras) and,
// once compiled, the classifier settings it scores with.
type Lexicon struct {
	Classes      map[model.Class]PhraseSet `toml:"classes"`
	Danger       []string                  `toml:"danger"`
	Codex        map[string][]string       `toml:"codex"`
	Continuation []string                  `toml:"continuation"`
	Context      []string                  `toml:"context"`
	Correction   []string                  `toml:"correction"`

	table []phrase
	cfg   model.ClassifierSettings
}

type phrase struct {
	text   string
	class  model.Class
	weight int
}

// Normalise applies NFKC and lower-casing so full- and half-width forms match.
func Normalise(s string) string { return strings.ToLower(norm.NFKC.String(s)) }

// Compile builds the phrase table: longest first, then the stronger weight,
// then class order, then text, so the result never depends on map order.
func (l *Lexicon) Compile(cfg model.ClassifierSettings) *Lexicon {
	best := map[string]phrase{}
	for _, class := range model.ClassOrder {
		set := l.Classes[class]
		for _, group := range []struct {
			list   []string
			weight int
		}{{set.Strong, cfg.StrongWeight}, {set.Weak, cfg.WeakWeight}} {
			for _, p := range group.list {
				n := Normalise(p)
				if cur, ok := best[n]; n != "" && (!ok || group.weight > cur.weight) {
					best[n] = phrase{n, class, group.weight}
				}
			}
		}
	}
	table := slices.Collect(maps.Values(best))
	slices.SortFunc(table, func(a, b phrase) int {
		return cmp.Or(
			cmp.Compare(utf8.RuneCountInString(b.text), utf8.RuneCountInString(a.text)),
			cmp.Compare(b.weight, a.weight),
			cmp.Compare(a.class.Rank(), b.class.Rank()),
			strings.Compare(a.text, b.text),
		)
	})
	out := *l
	out.table, out.cfg = table, cfg
	return &out
}

// Language returns ja, en, mixed or none.
func Language(s string) model.Language {
	cjk, latin := 0, 0
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han):
			cjk++
		case r < unicode.MaxASCII && unicode.IsLetter(r):
			latin++
		}
	}
	switch {
	case cjk == 0 && latin == 0:
		return model.LangNone
	case cjk > 0 && latin > 0 && min(cjk, latin)*4 >= max(cjk, latin):
		return model.LangMixed
	case cjk >= latin:
		return model.LangJA
	default:
		return model.LangEN
	}
}

// Hits returns the distinct normalised phrases found in text.
func Hits(text string, phrases []string) []string {
	return slices.Compact(slices.Sorted(fp.Filter(
		fp.Map(slices.Values(phrases), Normalise),
		func(p string) bool { return p != "" && strings.Contains(text, p) },
	)))
}

var traceRE = regexp.MustCompile(`(?im)(traceback \(most recent call last\)|^\s+at [\w.$<>]+\(.*:\d+\)|\b\w+(error|exception):|panic:|segmentation fault|fatal error)`)

// Signals are the rule-level features of a prompt (signal extraction is kept
// separate from the decision, as in vLLM Semantic Router, arXiv:2603.04444).
type Signals struct {
	Normalised   string
	Chars        int
	Lang         model.Language
	Scores       map[model.Class]int
	StrongHits   map[model.Class][]string
	Danger       []string
	Codex        model.CodexKind
	Continuation bool
	ContextCues  []string // "as discussed above": the task depends on history
	Correction   bool     // "no, that's wrong": the task is harder than it looks
	StackTrace   bool
	LongPrompt   bool
	CodeFence    bool
	// ClassCap is the per-class maximum the scores were capped at (features
	// normalise by it).
	ClassCap int
}

// ExtractSignals runs the rule stage: longest-match lexicon scoring plus
// structural signals measured in characters (never whitespace-separated words,
// which silently drops Japanese).
func (l *Lexicon) ExtractSignals(text string) Signals {
	normed := Normalise(text)
	stripped := strings.TrimSpace(normed)
	s := Signals{
		Normalised: stripped,
		Chars:      utf8.RuneCountInString(stripped),
		Lang:       Language(stripped),
		Scores:     map[model.Class]int{},
		StrongHits: map[model.Class][]string{},
		ClassCap:   l.cfg.ClassCap,
	}
	if stripped == "" {
		return s
	}
	s.Danger = Hits(normed, l.Danger)
	for _, kind := range []model.CodexKind{model.CodexReview, model.CodexTask} {
		if len(Hits(normed, l.Codex[string(kind)])) > 0 {
			s.Codex = kind
			break
		}
	}
	cfg := l.cfg
	s.Continuation = s.Chars <= cfg.ContinuationMaxChars && len(Hits(stripped, l.Continuation)) > 0
	s.ContextCues = Hits(normed, l.Context)
	s.Correction = len(Hits(normed, l.Correction)) > 0

	masked := normed
	for _, p := range l.table {
		if !strings.Contains(masked, p.text) {
			continue
		}
		// Longest match wins: mask the span so a shorter phrase cannot match
		// inside it again ("見直して" must not also count as "直して").
		masked = strings.ReplaceAll(masked, p.text, strings.Repeat("\x00", len(p.text)))
		s.Scores[p.class] = min(s.Scores[p.class]+p.weight, cfg.ClassCap)
		if p.weight == cfg.StrongWeight {
			s.StrongHits[p.class] = append(s.StrongHits[p.class], p.text)
		}
	}
	bump := func(c model.Class, by int) { s.Scores[c] = min(s.Scores[c]+by, cfg.ClassCap) }
	if s.CodeFence = strings.Contains(text, "```"); s.CodeFence {
		bump(model.ClassStandard, cfg.CodeFenceBonus)
	}
	if s.StackTrace = traceRE.MatchString(text); s.StackTrace {
		bump(model.ClassComplex, cfg.StackTraceBonus)
	}
	if s.LongPrompt = s.Chars >= cfg.LongPromptChars || strings.Count(text, "\n")+1 >= cfg.LongPromptLines; s.LongPrompt {
		bump(model.ClassArchitecture, cfg.LongPromptBonus)
		bump(model.ClassComplex, cfg.LongPromptBonus)
	}
	return s
}

// RuleClassify turns signals into a classification with the rule stage alone:
// the top class wins only above the confidence threshold and with a clear
// margin; otherwise it abstains (a wrong downgrade costs a retry).
func RuleClassify(s Signals, cfg model.ClassifierSettings) model.Classification {
	c := model.Classification{
		Scores:       s.Scores,
		Danger:       len(s.Danger) > 0,
		Codex:        s.Codex,
		Continuation: s.Continuation,
		Lang:         s.Lang,
		Chars:        s.Chars,
		Reasons:      reasons(s, cfg),
	}
	if s.Chars == 0 {
		c.Reasons = append(c.Reasons, "empty")
		return c
	}
	ranked := slices.SortedStableFunc(slices.Values(model.ClassOrder), func(a, b model.Class) int {
		// Higher score first; on a tie the more demanding class ranks first.
		return cmp.Or(cmp.Compare(s.Scores[b], s.Scores[a]), cmp.Compare(b.Rank(), a.Rank()))
	})
	top, second := ranked[0], ranked[1]
	c.Confidence = s.Scores[top]
	c.Margin = s.Scores[top] - s.Scores[second]
	threshold := cfg.MinConfidence
	switch {
	case c.Confidence < threshold:
		c.Reasons = append(c.Reasons, "below-threshold")
	case c.Margin < cfg.MinMargin && c.Confidence < cfg.ConfidentMultiple*threshold:
		c.Reasons = append(c.Reasons, "ambiguous:"+string(top)+"/"+string(second))
	default:
		c.Class = top
	}
	return c
}

func reasons(s Signals, cfg model.ClassifierSettings) []string {
	var out []string
	if len(s.Danger) > 0 {
		out = append(out, "danger:"+strings.Join(s.Danger[:min(cfg.MaxDangerHits, len(s.Danger))], ","))
	}
	if s.Continuation {
		out = append(out, "continuation")
	}
	for _, c := range model.ClassOrder {
		if h := s.StrongHits[c]; len(h) > 0 {
			out = append(out, string(c)+":"+strings.Join(h[:min(cfg.MaxReasonHits, len(h))], ","))
		}
	}
	if len(s.ContextCues) > 0 {
		out = append(out, "context-dependent")
	}
	if s.StackTrace {
		out = append(out, "stack-trace")
	}
	if s.LongPrompt {
		out = append(out, "long-prompt")
	}
	return out
}
