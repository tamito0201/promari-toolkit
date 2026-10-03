package model

// Language of a prompt.
type Language string

// Detected languages.
const (
	LangNone  Language = "none"
	LangJA    Language = "ja"
	LangEN    Language = "en"
	LangMixed Language = "mixed"
)

// CodexKind says what kind of Codex hand-off a prompt suits.
type CodexKind string

// Codex suitability.
const (
	CodexNone   CodexKind = ""
	CodexReview CodexKind = "review"
	CodexTask   CodexKind = "task"
)

// Classification is the immutable outcome of classifying one prompt.
type Classification struct {
	Class        Class
	Confidence   int
	Margin       int
	Scores       map[Class]int
	Danger       bool
	Codex        CodexKind
	Continuation bool
	Lang         Language
	Chars        int
	Reasons      []string
}

// Score returns the score of a class (0 when absent).
func (c Classification) Score(class Class) int { return c.Scores[class] }
