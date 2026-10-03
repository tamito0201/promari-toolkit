package service

import (
	"slices"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

// TestComposeHabits covers the 🎓 Habits category.
func TestComposeHabits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		view   View
		want   string
		alarms []string
	}{
		{
			"a branch that broke every habit",
			View{Now: now, Facts: model.Facts{
				Git: model.Some(model.Git{
					Branch: "login", DefaultBranch: "main", BranchStart: now.Add(-30 * time.Hour), OldestUnpushed: now.Add(-26 * time.Hour),
					MergedBranches: 2, ConflictMarkers: 3, Junk: 2, JunkStaged: 1, CommitsToday: 7, TodayAdded: 1500, TodayDeleted: 100,
					LargestToday: 900, BehindDefault: 12, ConventionalToday: 2, VagueToday: 3, Streak: 1, LongestStreak: 4,
				}),
				Pull:    model.Some(model.PullRequest{Number: 3, Created: now.Add(-2 * time.Hour), PeopleKnown: true}),
				Reviews: model.Some(model.ReviewQueue{Count: 2, Oldest: now.Add(-3 * time.Hour)}),
			}},
			"🎓 Habits: Name ✗ kind/topic | Branch 30h00m | Unpushed 26h00m | Merged ×2 left | Conflict marks ×3 | Junk ×2 | 229 L/cmt ×7/day max 900 +/- 15.0 | Behind main ×12 | Conv 2/7 | Vague msg ×3 | Streak 1d (max 4d) | No assignee | No reviewer | Review wait 2h00m | To review ×2 (oldest 3h00m)",
			[]string{"Unpushed 26h00m", "Conflict marks ×3"},
		},
		{
			"working on the default branch, and good habits elsewhere",
			View{Now: now, Facts: model.Facts{
				Git: model.Some(model.Git{
					Branch: "main", DefaultBranch: "main", Changed: 2, CommitsToday: 3, TodayAdded: 90, TodayDeleted: 15,
					ConventionalToday: 3, Streak: 5, LongestStreak: 5,
				}),
				Pull:    model.Some(model.PullRequest{Number: 3, Assignees: 1, Reviews: 1, Created: now.Add(-time.Hour), PeopleKnown: true}),
				Reviews: model.Some(model.ReviewQueue{Count: 1, Oldest: now.Add(-10 * time.Minute)}),
			}},
			"🎓 Habits: On main directly | 35 L/cmt ×3/day +/- 6.0 | Conv 3/3 | Streak 5d (max 5d) | To review ×1 (oldest 10m)",
			[]string{"On main directly"},
		},
		{
			"a young branch, a draft, a queue without its age, people unknown",
			View{Now: now, Facts: model.Facts{
				Git:     model.Some(model.Git{Branch: "feature/login-ui", DefaultBranch: "main", BranchStart: now.Add(-time.Hour), CommitsToday: 1, TodayAdded: 3}),
				Pull:    model.Some(model.PullRequest{Number: 3, Draft: true, PeopleKnown: true, Assignees: 1}),
				Reviews: model.Some(model.ReviewQueue{Count: 1}),
			}},
			"🎓 Habits: Branch 1h00m | 3 L/cmt ×1/day | To review ×1",
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			groups := Compose(&tt.view)
			got := render(groups)
			i := slices.IndexFunc(got, func(s string) bool { return strings.HasPrefix(s, "🎓 Habits") })
			if i < 0 || got[i] != tt.want {
				t.Errorf("Compose() =\n%s\nwant\n%s", strings.Join(got, "\n"), tt.want)
			}
			if a := alarms(groups); !slices.Equal(a, tt.alarms) {
				t.Errorf("alarms = %q, want %q", a, tt.alarms)
			}
		})
	}
	unknown := View{Now: now, Facts: model.Facts{Git: model.Some(model.Git{Branch: "x"}), Pull: model.Some(model.PullRequest{Number: 3})}}
	if slices.ContainsFunc(render(Compose(&unknown)), func(s string) bool { return strings.HasPrefix(s, "🎓") }) {
		t.Error("a branch of a repository without a known default and a pull request whose people are unknown show no habits")
	}
}
