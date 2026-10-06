package dashboard

import (
	"strings"
	"testing"

	"promari-statusline/internal/domain/model"
)

// TestNoteWordsEveryBasis words each kind of basis and each scope: every
// observed scope has its own words, and a scope with a window names it.
func TestNoteWordsEveryBasis(t *testing.T) {
	t.Parallel()
	windowed := map[model.Scope]bool{
		model.ScopeRecentTurns: true, model.ScopeNearestRank: true,
		model.ScopeRoundTrips: true, model.ScopeClosedPrompts: true,
	}
	seen := map[string]model.Scope{}
	for s := model.ScopeRecentTurns; s <= model.ScopeRecognizedRuns; s++ {
		got := note(model.Basis{Kind: model.BasisObserved, Scope: s, N: 7, Window: 123})
		if !strings.HasSuffix(got, " · n=7") || got == " · n=7" {
			t.Errorf("scope %d: %q", s, got)
		}
		if strings.Contains(got, "123") != windowed[s] {
			t.Errorf("scope %d names the window %v: %q", s, !windowed[s], got)
		}
		if other, dup := seen[got]; dup {
			t.Errorf("scopes %d and %d share the words %q", other, s, got)
		}
		seen[got] = s
	}
	for _, c := range []struct {
		basis model.Basis
		want  string
	}{
		{model.Basis{Kind: model.BasisCounted}, ""},
		{model.Basis{Kind: model.BasisObserved, Scope: model.ScopeNone, N: 3}, " · n=3"},
		{model.Basis{Kind: model.BasisWaiting, N: 4, Required: 20}, "観測 4 / 必要 20 件"},
		{model.Basis{Kind: model.BasisNoBlock}, "稼働枠なし"},
		{model.Basis{Kind: model.BasisTooManyKinds, Limit: 128}, "ツール種類数が追跡上限128を超過"},
	} {
		if got := note(c.basis); got != c.want {
			t.Errorf("note(%+v) = %q, want %q", c.basis, got, c.want)
		}
	}
}

// TestToneNameNamesEveryTone gives every tone but the plain one its own name.
func TestToneNameNamesEveryTone(t *testing.T) {
	t.Parallel()
	seen := map[string]model.Tone{}
	for _, tone := range []model.Tone{
		model.ToneAccent, model.ToneGood, model.ToneCaution, model.ToneDanger, model.ToneInfo,
		model.ToneMoney, model.ToneNote, model.ToneMuted, model.ToneBrand,
	} {
		name := toneName(tone)
		if other, dup := seen[name]; name == "" || dup {
			t.Errorf("tone %d is named %q (also tone %d)", tone, name, other)
		}
		seen[name] = tone
	}
	if got := toneName(model.TonePlain); got != "" {
		t.Errorf("the plain tone is named %q", got)
	}
}
