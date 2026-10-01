package service

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/pkg/fp"
)

// AdviceInput is everything the advice for one user prompt depends on.
type AdviceInput struct {
	Classification model.Classification
	Signals        Signals
	Session        model.SessionModel
	Settings       model.Settings
	Pressure       model.Pressure
	Codex          model.CodexQuota
}

// Render fills a message template (text/template) with data. A broken
// template degrades to the raw text instead of failing the hook.
func Render(tmpl string, data any) string {
	t, err := template.New("m").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return tmpl
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return tmpl
	}
	return b.String()
}

// note is one piece of advice; None means "nothing to say".
type note = func(AdviceInput) fp.Option[string]

func when(cond bool, text string) fp.Option[string] {
	if cond {
		return fp.Some(text)
	}
	return fp.None[string]()
}

// workNotes are evaluated in order; the first that applies is used. The Codex
// and usage notes are added independently.
var workNotes = []note{
	func(in AdviceInput) fp.Option[string] {
		return when(in.Classification.Danger, in.Settings.Messages.Danger)
	},
	func(in AdviceInput) fp.Option[string] {
		// Harness Tokenomics (arXiv:2609.28919): a correction means the task is
		// harder than it looks; never treat it as cheap.
		return when(in.Signals.Correction, in.Settings.Messages.Correction)
	},
	func(in AdviceInput) fp.Option[string] {
		t, c := in.Session.Tier(), in.Classification.Class
		return when(c.Deep() && model.TierOpus.Above(t), Render(in.Settings.Messages.DeepOnWeakSession,
			map[string]string{"Class": string(c), "Session": string(t)}))
	},
	func(in AdviceInput) fp.Option[string] {
		return when(in.Classification.Class == model.ClassLookup && in.Session.Tier().Above(model.TierHaiku), in.Settings.Messages.Lookup)
	},
	func(in AdviceInput) fp.Option[string] {
		return when(in.Classification.Class == model.ClassMechanical && in.Classification.Codex == model.CodexTask, in.Settings.Messages.BulkMechanical)
	},
}

func codexNote(in AdviceInput) fp.Option[string] {
	c, cx, msg := in.Classification, in.Settings.Codex, in.Settings.Messages
	if c.Codex == model.CodexNone || !cx.Enabled || c.Danger {
		return fp.None[string]()
	}
	if used, ok := in.Codex.Used.Get(); ok && !in.Codex.Available {
		return fp.Some(Render(msg.CodexBlocked, map[string]float64{"Used": used}))
	}
	if c.Codex == model.CodexReview {
		return fp.Some(msg.CodexReview)
	}
	return fp.Some(Render(msg.CodexTask, map[string]string{"Model": cx.Model, "Effort": cx.Effort}))
}

func pressureNote(in AdviceInput) fp.Option[string] {
	if !in.Pressure.High {
		return fp.None[string]()
	}
	parts := collect(
		fp.MapOption(in.Pressure.FiveHour, func(v float64) string { return fmt.Sprintf("5h %.0f%%", v) }),
		fp.MapOption(in.Pressure.SevenDay, func(v float64) string { return fmt.Sprintf("7d %.0f%%", v) }),
	)
	return fp.Some(Render(in.Settings.Messages.Pressure, map[string]string{"Usage": strings.Join(parts, " ")}))
}

func collect(opts ...fp.Option[string]) []string {
	var out []string
	for _, o := range opts {
		if v, ok := o.Get(); ok && v != "" {
			out = append(out, v)
		}
	}
	return out
}

// Advise returns the advice for a user prompt, or "". It never blocks the
// prompt: routers that stop it with exit 2 force a resend on every turn.
func Advise(in AdviceInput) string {
	c, cfg := in.Classification, in.Settings.Advice
	if !cfg.Enabled || c.Chars < cfg.MinPromptChars {
		return ""
	}
	// A short continuation ("続けて") gets no advice, except the safety note:
	// "続けて。本番のパスワードも変えて" must still warn.
	if c.Continuation {
		if !c.Danger {
			return ""
		}
		return Truncate(in.Settings.Messages.Prefix+" "+in.Settings.Messages.Danger, cfg.MaxChars)
	}
	notes := collect(fp.FirstSome(fp.Apply(workNotes, in)...), codexNote(in), pressureNote(in))
	if len(notes) == 0 {
		return ""
	}
	return Truncate(in.Settings.Messages.Prefix+" "+strings.Join(notes, " "), cfg.MaxChars)
}

// Truncate cuts text to at most limit runes, marking the cut with "…".
func Truncate(text string, limit int) string {
	if r := []rune(text); limit > 0 && len(r) > limit {
		return string(r[:limit-1]) + "…"
	}
	return text
}

// SessionContext is what SessionStart injects: the routing policy text.
func SessionContext(m model.Messages) string { return m.Prefix + " " + m.SessionPolicy }

// ResolveSession picks the most reliable observation of the session model:
// the transcript (what actually ran), then the recorded session state, then
// ANTHROPIC_MODEL, then settings. The first two are certain enough to cap a
// route; the others may be stale because --model and /model outrank them.
func ResolveSession(observations ...fp.Option[model.SessionModel]) model.SessionModel {
	for _, o := range observations {
		if v, ok := o.Get(); ok && v.Model != "" {
			return v
		}
	}
	return model.SessionModel{Source: model.SourceUnknown}
}
