package service

import (
	"slices"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

// TestComposeDue covers the ⏰ Due category.
func TestComposeDue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		w      model.Workload
		want   string
		alarms []string
	}{
		{
			"late, urgent, a red default branch and everything owed",
			model.Workload{
				Issues: 200, IssuesCapped: true, Overdue: 2, MostLate: 5, LatestIssue: 85, Urgent: 1, DueToday: 1, DueWeek: 3, Undated: 150,
				Stale: 4, Open: 3, Drafts: 1, MyTurn: 1, Merged: 100, MergedCapped: true, LeadP50: 6 * time.Hour,
				Reviewed: 12, FirstPass: 5, Abandoned: 2, Red: 2, RedWorkflow: "CI", RedSince: now.Add(-40 * time.Minute),
			},
			"⏰ Due: Overdue ×2 (#85 5d) | Urgent >48h ×1 | Base ✗ CI 40m (+1 more) | Due today ×1 | Due 7d ×3 | My turn ×1 | Stale 14d+ ×4 | Undated 150/200+ | WIP 3 PR (1 draft) | Lead p50 6h00m ×100+/14d | 1st-pass 5/12 | Abandoned ×2/14d",
			[]string{"Base ✗ CI 40m (+1 more)"},
		},
		{
			"few reviews are not coloured, a fresh red does not blink",
			model.Workload{Issues: 1, Undated: 1, Merged: 3, LeadP50: 20 * time.Minute, Reviewed: 3, Red: 1, RedWorkflow: "CI", RedSince: now.Add(-5 * time.Minute)},
			"⏰ Due: Base ✗ CI 5m | Undated 1/1 | Lead p50 20m ×3/14d | 1st-pass 0/3",
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			view := View{Now: now, Facts: model.Facts{Workload: model.Some(tt.w)}}
			groups := Compose(&view)
			got := render(groups)
			i := slices.IndexFunc(got, func(s string) bool { return strings.HasPrefix(s, "⏰ Due") })
			if i < 0 || got[i] != tt.want {
				t.Errorf("Compose() =\n%s\nwant\n%s", strings.Join(got, "\n"), tt.want)
			}
			if a := alarms(groups); !slices.Equal(a, tt.alarms) {
				t.Errorf("alarms = %q, want %q", a, tt.alarms)
			}
		})
	}
	nothing := View{Now: now, Facts: model.Facts{Workload: model.Some(model.Workload{})}}
	if slices.ContainsFunc(render(Compose(&nothing)), func(s string) bool { return strings.HasPrefix(s, "⏰") }) {
		t.Error("nothing owed shows nothing: the exceptions only")
	}
}
