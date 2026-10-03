package service

import (
	"slices"
	"strings"
	"testing"

	"promari-statusline/internal/domain/model"
)

// TestComposeRules covers the 📏 Rules category.
func TestComposeRules(t *testing.T) {
	t.Parallel()
	var broken model.Violations
	broken[model.RuleSQLConcat] = 1
	broken[model.RuleEmptyCatch] = 2
	broken[model.RuleNoCSRF] = 1
	broken[model.RuleLongLine] = 4
	tests := []struct {
		name   string
		git    model.Git
		want   string
		alarms []string
	}{
		{
			"rules broken, the most serious first",
			model.Git{Branch: "feature/order", Rules: broken},
			"📏 Rules: SQL+str ×1 | Empty catch ×2 | No CSRF ×1 | >120 col ×4",
			[]string{"SQL+str ×1", "Empty catch ×2"},
		},
		{
			"nothing broken in what was read, and lines left unread",
			model.Git{Branch: "feature/order", RulesUnchecked: 1200},
			"📏 Rules: Unchecked 1,200 lines",
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			view := View{Now: now, Facts: model.Facts{Git: model.Some(tt.git)}}
			groups := Compose(&view)
			got := render(groups)
			i := slices.IndexFunc(got, func(s string) bool { return strings.HasPrefix(s, "📏 Rules") })
			if i < 0 || got[i] != tt.want {
				t.Errorf("Compose() =\n%s\nwant\n%s", strings.Join(got, "\n"), tt.want)
			}
			if a := alarms(groups); !slices.Equal(a, tt.alarms) {
				t.Errorf("alarms = %q, want %q", a, tt.alarms)
			}
		})
	}
	clean := View{Now: now, Facts: model.Facts{Git: model.Some(model.Git{Branch: "x"})}}
	if slices.ContainsFunc(render(Compose(&clean)), func(s string) bool { return strings.HasPrefix(s, "📏") }) {
		t.Error("a change that breaks no rule shows no rules")
	}
}
