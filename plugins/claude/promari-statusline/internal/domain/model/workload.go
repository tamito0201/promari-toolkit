package model

import (
	"regexp"
	"slices"
	"time"
)

// The work a developer owes, measured the way the KPI books measure it: the
// exceptions only, now and in the next days, not monthly averages. David
// Parmenter's "Key Performance Indicators" (2nd and 4th editions) has the late
// projects reported weekly as its most common KPI, measures "past, current and
// future", and thresholds such as enquiries not answered for 24 hours, requests
// outstanding for 48 hours and complaints still unresolved after two weeks.
// Bernie Smith's "KPI Checklists" asks for 10 to 30 points before a trend is
// judged, and Nishtha Duggal's "Mastering Customer Support" for the time to
// resolve as a lead time from report to close.
const (
	// StaleAfter is how long an assigned issue or an own pull request may go
	// without an update: "still unresolved after two weeks" (Parmenter).
	StaleAfter = 14 * 24 * time.Hour
	// UrgentAfter is how long an urgent issue may stay open: "requests
	// outstanding for more than 48 hours" (Parmenter).
	UrgentAfter = 48 * time.Hour
	// DueSoon is the horizon of the future measures: what falls due within a
	// week.
	DueSoon = 7 * 24 * time.Hour
	// FlowWindow is the period of the past measures: the pull requests merged
	// or closed in the last two weeks.
	FlowWindow = 14 * 24 * time.Hour
	// MinJudged is the fewest points a rate is coloured on: "at least 10 to 30
	// representative data points" (Smith).
	MinJudged = 10
)

// urgentLabel matches the labels that mark an issue as urgent.
var urgentLabel = regexp.MustCompile(`(?i)^(?:p0|p1|sev[-:]?[01]|critical|urgent|blocker|incident|hotfix|priority[-:/ ]*(?:critical|urgent|highest|p0|p1))$`)

// IsUrgentLabel reports whether a label marks an issue as urgent.
func IsUrgentLabel(label string) bool { return urgentLabel.MatchString(label) }

// IssueItem is an open issue assigned to the user.
type IssueItem struct {
	Number  int
	Updated time.Time
	Created time.Time
	// Due is the due date of the issue's milestone, a calendar day; zero when
	// it has none.
	Due    time.Time
	Labels []string
}

// PullItem is a pull request of the user's.
type PullItem struct {
	Number  int
	Updated time.Time
	Created time.Time
	Merged  time.Time
	Draft   bool
	// ChangesRequested is a review asking for changes: the ball is the
	// author's.
	ChangesRequested bool
	// Reviewed is a pull request with a review; FirstPass one whose first
	// review approved it.
	Reviewed, FirstPass bool
}

// RunItem is a run of a workflow on the default branch, newest first.
type RunItem struct {
	Workflow   string
	Conclusion string
	Created    time.Time
}

// Workload is what is owed, what is late and how the finished work flowed.
type Workload struct {
	// Issues are the open issues assigned to the user. Overdue are those whose
	// due day has passed, MostLate how many days the latest is late and
	// LatestIssue its number. DueToday fall due today, DueWeek within
	// DueSoon, Undated have no due day at all.
	Issues      int `json:"issues,omitzero"`
	Overdue     int `json:"overdue,omitzero"`
	MostLate    int `json:"most_late,omitzero"`
	LatestIssue int `json:"latest_issue,omitzero"`
	DueToday    int `json:"due_today,omitzero"`
	DueWeek     int `json:"due_week,omitzero"`
	Undated     int `json:"undated,omitzero"`
	// Stale are the assigned issues and own open pull requests without an
	// update for StaleAfter; Urgent the urgent issues open for UrgentAfter.
	Stale  int `json:"stale,omitzero"`
	Urgent int `json:"urgent,omitzero"`

	// Open are the user's open pull requests, Drafts of them drafts and MyTurn
	// those whose review asks for changes.
	Open   int `json:"open,omitzero"`
	Drafts int `json:"drafts,omitzero"`
	MyTurn int `json:"my_turn,omitzero"`

	// Merged are the user's pull requests merged within FlowWindow, LeadP50
	// the median from opening to merging, Reviewed those with a review and
	// FirstPass those approved by their first review. Abandoned were closed
	// without merging in the same window.
	Merged    int           `json:"merged,omitzero"`
	LeadP50   time.Duration `json:"lead_p50,omitzero"`
	Reviewed  int           `json:"reviewed,omitzero"`
	FirstPass int           `json:"first_pass,omitzero"`
	Abandoned int           `json:"abandoned,omitzero"`

	// RedSince is when the failing workflow of the default branch began to
	// fail, RedWorkflow its name; zero while every workflow's last run passed.
	RedSince    time.Time `json:"red_since,omitzero"`
	RedWorkflow string    `json:"red_workflow,omitzero"`
	Red         int       `json:"red,omitzero"`

	// IssuesCapped and MergedCapped are lists that reached the most gh was
	// asked for: their counts are lower bounds.
	IssuesCapped bool `json:"issues_capped,omitzero"`
	MergedCapped bool `json:"merged_capped,omitzero"`
}

// day returns the calendar day of t in loc, at midnight.
func day(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// AddIssues counts the assigned issues. A due day is a calendar day: GitHub
// keeps a milestone's due date as midnight UTC of the day chosen, so its UTC
// date is compared with today's local date.
func (w *Workload) AddIssues(items []IssueItem, now time.Time) {
	today := day(now, now.Location())
	for _, it := range items {
		w.Issues++
		if !it.Updated.IsZero() && now.Sub(it.Updated) > StaleAfter {
			w.Stale++
		}
		if !it.Created.IsZero() && now.Sub(it.Created) > UrgentAfter && slices.ContainsFunc(it.Labels, IsUrgentLabel) {
			w.Urgent++
		}
		if it.Due.IsZero() {
			w.Undated++
			continue
		}
		y, m, d := it.Due.UTC().Date()
		due := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
		switch late := int(today.Sub(due).Hours() / 24); {
		case late > 0:
			w.Overdue++
			if late > w.MostLate {
				w.MostLate, w.LatestIssue = late, it.Number
			}
		case late == 0:
			w.DueToday++
		case due.Sub(today) <= DueSoon:
			w.DueWeek++
		}
	}
}

// AddOpen counts the user's open pull requests.
func (w *Workload) AddOpen(items []PullItem, now time.Time) {
	for _, it := range items {
		w.Open++
		if it.Draft {
			w.Drafts++
		}
		if it.ChangesRequested {
			w.MyTurn++
		}
		if !it.Updated.IsZero() && now.Sub(it.Updated) > StaleAfter {
			w.Stale++
		}
	}
}

// AddMerged takes the user's pull requests merged within FlowWindow and
// finds their median lead time. The median, not the mean: one pull request
// left open for a month would move the mean by days ("Beware of averages").
func (w *Workload) AddMerged(items []PullItem) {
	var leads []time.Duration
	for _, it := range items {
		if it.Merged.IsZero() || it.Created.IsZero() {
			continue
		}
		w.Merged++
		leads = append(leads, it.Merged.Sub(it.Created))
		if it.Reviewed {
			w.Reviewed++
			if it.FirstPass {
				w.FirstPass++
			}
		}
	}
	if len(leads) > 0 {
		slices.Sort(leads)
		w.LeadP50 = leads[len(leads)/2]
		if len(leads)%2 == 0 {
			w.LeadP50 = (leads[len(leads)/2-1] + leads[len(leads)/2]) / 2
		}
	}
}

// AddRuns counts the workflows of the default branch whose last run failed,
// and finds since when the oldest has failed: the first of its failures in a
// row. Runs not yet
// concluded are skipped.
func (w *Workload) AddRuns(runs []RunItem) {
	seen := map[string]bool{}
	for i, r := range runs {
		if r.Conclusion == "" || seen[r.Workflow] {
			continue
		}
		seen[r.Workflow] = true
		if !isFailure(r.Conclusion) {
			continue
		}
		w.Red++
		since := r.Created
		for _, older := range runs[i+1:] {
			if older.Workflow != r.Workflow || older.Conclusion == "" {
				continue
			}
			if !isFailure(older.Conclusion) {
				break
			}
			since = older.Created
		}
		if w.RedSince.IsZero() || since.Before(w.RedSince) {
			w.RedSince, w.RedWorkflow = since, r.Workflow
		}
	}
}

func isFailure(conclusion string) bool {
	return slices.Contains([]string{"failure", "timed_out", "startup_failure"}, conclusion)
}
