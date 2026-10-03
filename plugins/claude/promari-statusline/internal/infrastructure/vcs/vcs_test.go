package vcs_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform/platformtest"
	"promari-statusline/internal/infrastructure/vcs"
)

var t0 = time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC)

var errExit = errors.New("exit status 128")

// todayLog is today's commits: two co-authored by an AI, one of them a
// revert, and a fix; a line that is no record is skipped.
const todayLog = "\x1efeat: a\x1fClaude <claude@example.com>\x1f\n\n30\t10\ta.go\n-\t-\timg.png\n" +
	"\x1efix(x): b\x1f\x1f\n\n5\t5\tb.go\n\x1eRevert \"feat: a\"\x1fA <a@x>,Copilot <c@x>\x1f\n\x1edocs: 修正\x1f\x1f\nnot a record\n"

// mergingDiff adds and removes a debt, weakens a test (a skip added, two
// assertions removed, a mock added) and adds dependencies to two manifests,
// with the package's own version left out.
const mergingDiff = "1\t1\tm.go\n3\t2\tm_test.go\n2\t0\tgo.mod\n3\t0\tpackage.json\n4\t0\tSvc.cs\n" +
	"diff --git a/m.go b/m.go\n--- a/m.go\n+++ b/m.go\n@@ -1 +1 @@\n-// FIXME: old\n+// TODO: later\n" +
	"diff --git a/m_test.go b/m_test.go\n--- a/m_test.go\n+++ b/m_test.go\n@@ -1,2 +1,3 @@\n" +
	"-\tassert.Equal(t, 1, got)\n-\tt.Errorf(\"bad\")\n+\tt.Skip(\"later\")\n+\tstore := mocks.NewStore(t)\n+\t_ = store\n" +
	"diff --git a/go.mod b/go.mod\n--- a/go.mod\n+++ b/go.mod\n@@ -3,0 +4,2 @@\n+\tgithub.com/x/y v1.2.3\n+// a comment\n" +
	"diff --git a/package.json b/package.json\n--- a/package.json\n+++ b/package.json\n@@ -2,0 +3,3 @@\n+  \"version\": \"1.5.0\",\n+  \"left-pad\": \"^1.3.0\",\n+  \"@scope/pkg\": \"workspace:*\",\n" +
	"diff --git a/Svc.cs b/Svc.cs\n--- a/Svc.cs\n+++ b/Svc.cs\n@@ -9,0 +10,4 @@\n+    catch (IOException e)\n+    {\n+    }\n+    throw ex;\n"

// broken returns the rules broken once each.
func broken(rules ...model.Rule) model.Violations {
	var v model.Violations
	for _, r := range rules {
		v[r]++
	}
	return v
}

func TestGit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cmds map[string]platformtest.Result
		want model.Git
		err  error
	}{
		{
			"a branch with everything to report",
			map[string]platformtest.Result{
				"git --no-optional-locks -C /work branch --show-current":                            {Out: "develop\n"},
				"git --no-optional-locks -C /work status --porcelain":                               {Out: " M a.go\n?? b.go\n\n"},
				"git --no-optional-locks -C /work stash list":                                       {Out: "stash@{0}: x\nstash@{1}: y\n"},
				"git --no-optional-locks -C /work rev-list --left-right --count @{upstream}...HEAD": {Out: "16\t3\n"},
				"git --no-optional-locks -C /work log -1 --format=%ct":                              {Out: "1759464540\n"},
			},
			model.Git{Branch: "develop", Changed: 2, Untracked: 1, Stashes: 2, Behind: 16, Ahead: 3, LastCommit: time.Unix(1759464540, 0)},
			nil,
		},
		{
			"the files broken down, the lines changed, a rebase and the commits of today",
			map[string]platformtest.Result{
				"git --no-optional-locks -C /work branch --show-current":                                                                                              {Out: "develop\n"},
				"git --no-optional-locks -C /work status --porcelain":                                                                                                 {Out: "M  staged.go\nMM both.go\n M work.go\nUU conflict.go\nAA added.go\n?? new.go\nR  old.go -> new2.go\nX\n"},
				"git --no-optional-locks -C /work diff HEAD --numstat --patch --unified=0 --no-color --no-ext-diff --no-renames":                                      {Out: "10\t30\ta.go\n2\t0\tb_test.go\n-\t-\timg.png\nx\n"},
				"git --no-optional-locks -C /work rev-parse --absolute-git-dir":                                                                                       {Out: "/work/.git\n"},
				"git --no-optional-locks -C /work log --since=midnight --numstat --format=%x1e%s%x1f%(trailers:key=Co-authored-by,valueonly,separator=%x2C)%x1f HEAD": {Out: todayLog},
			},
			model.Git{
				Branch: "develop", Changed: 8, Staged: 3, Untracked: 1, Conflicts: 2, Inserted: 12, Deleted: 30,
				Operation: "rebase", CommitsToday: 4, FixesToday: 2, AICommitsToday: 2, TodayAdded: 35, TodayDeleted: 15, VagueToday: 3, LargestToday: 40, ConventionalToday: 3, // a, b and 修正 say nothing after their prefix
				Changes: []model.FileChange{{Path: "a.go", Added: 10, Deleted: 30}, {Path: "b_test.go", Added: 2}, {Path: "img.png"}},
			},
			nil,
		},
		{
			"a merge in progress, only insertions",
			map[string]platformtest.Result{
				"git --no-optional-locks -C /merging branch --show-current":                                                         {Out: "develop\n"},
				"git --no-optional-locks -C /merging diff HEAD --numstat --patch --unified=0 --no-color --no-ext-diff --no-renames": {Out: mergingDiff},
				"git --no-optional-locks -C /merging rev-parse --absolute-git-dir":                                                  {Out: "/merging/.git\n"},
			},
			model.Git{
				Branch: "develop", Inserted: 13, Deleted: 3, Operation: "merge",
				Changes:   []model.FileChange{{Path: "m.go", Added: 1, Deleted: 1}, {Path: "m_test.go", Added: 3, Deleted: 2}, {Path: "go.mod", Added: 2}, {Path: "package.json", Added: 3}, {Path: "Svc.cs", Added: 4}},
				DebtAdded: 1, DebtRemoved: 1, MocksAdded: 1, SkipsAdded: 1, AssertsRemoved: 2, DepsAdded: 3,
				Rules: broken(model.RuleEmptyCatch, model.RuleThrowEx),
			},
			nil,
		},
		{
			"a new branch: no upstream, no commit, nothing changed",
			map[string]platformtest.Result{
				"git --no-optional-locks -C /work branch --show-current":                            {Out: "feature\n"},
				"git --no-optional-locks -C /work rev-list --left-right --count @{upstream}...HEAD": {Err: errExit},
				"git --no-optional-locks -C /work log -1 --format=%ct":                              {Err: errExit},
			},
			model.Git{Branch: "feature"},
			nil,
		},
		{
			"counts that are not numbers are left at zero",
			map[string]platformtest.Result{
				"git --no-optional-locks -C /work branch --show-current":                            {Out: "feature\n"},
				"git --no-optional-locks -C /work rev-list --left-right --count @{upstream}...HEAD": {Out: "x y\n"},
				"git --no-optional-locks -C /work log -1 --format=%ct":                              {Out: "yesterday\n"},
			},
			model.Git{Branch: "feature"},
			nil,
		},
		{"a detached HEAD has no branch", map[string]platformtest.Result{"git --no-optional-locks -C /work branch --show-current": {Out: "\n"}}, model.Git{}, repository.ErrNone},
		{"not a repository, or no git", nil, model.Git{}, repository.ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Cmds = tt.cmds
			// Each git directory carries the mark of an operation in progress.
			sys.Files["/work/.git/rebase-merge"] = nil
			sys.Files["/merging/.git/MERGE_HEAD"] = nil
			dir := "/work"
			for cmd := range tt.cmds {
				dir, _, _ = strings.Cut(strings.TrimPrefix(cmd, "git --no-optional-locks -C "), " ")
			}
			got, err := vcs.Git{Sys: sys}.Git(context.Background(), dir)
			if !reflect.DeepEqual(got, tt.want) || !errors.Is(err, tt.err) {
				t.Errorf("Git() = %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestGitHabits(t *testing.T) {
	t.Parallel()
	const at = "git --no-optional-locks -C /work "
	branchStart, unpushed := t0.Add(-5*time.Hour), t0.Add(-30*time.Hour)
	unix := func(ts ...time.Time) string {
		var b strings.Builder
		for _, x := range ts {
			b.WriteString(strconv.FormatInt(x.Unix(), 10) + "\n")
		}
		return b.String()
	}
	tests := []struct {
		name string
		cmds map[string]platformtest.Result
		want func(model.Git) bool
	}{
		{
			"a branch of its own: its age, the unpushed commits, merged branches left, the streak, junk and a conflict marker",
			map[string]platformtest.Result{
				at + "branch --show-current": {Out: "feature/login\n"},
				at + "status --porcelain":    {Out: "?? obj/\nA  .env\n M a.go\n"},
				at + "diff HEAD --numstat --patch --unified=0 --no-color --no-ext-diff --no-renames": {Out: "2\t0\ta.go\ndiff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1,2 @@\n+<<<<<<< HEAD\n+=======\n"},
				at + "rev-list --left-right --count @{upstream}...HEAD":                              {Out: "0\t2\n"},
				at + "symbolic-ref --short refs/remotes/origin/HEAD":                                 {Out: "origin/main\n"},
				at + "log --format=%ct origin/main..HEAD":                                            {Out: unix(t0.Add(-time.Hour), branchStart)},
				at + "branch --merged main --format=%(refname:short)":                                {Out: "main\nfeature/login\nfix/old\ndocs/a\n"},
				at + "log --format=%ct @{upstream}..HEAD":                                            {Out: unix(unpushed, t0)},
				at + "rev-parse --abbrev-ref @{upstream}":                                            {Out: "origin/feature/login\n"},
				at + "config user.email":                                                             {Out: "me@example.com\n"},
				at + "log --since=60.days --author=me@example.com --format=%cs HEAD":                 {Out: "2026-10-03\n2026-10-02\n2026-10-02\n2026-09-20\n"},
			},
			func(g model.Git) bool {
				return g.DefaultBranch == "main" && g.BranchStart.Equal(branchStart) && g.OldestUnpushed.Equal(unpushed) &&
					g.MergedBranches == 2 && g.Streak == 2 && g.LongestStreak == 2 && g.Junk == 2 && g.JunkStaged == 1 && g.ConflictMarkers == 2
			},
		},
		{
			"a branch never pushed waits from its first commit",
			map[string]platformtest.Result{
				at + "branch --show-current":                         {Out: "feature/x\n"},
				at + "symbolic-ref --short refs/remotes/origin/HEAD": {Out: "origin/main\n"},
				at + "log --format=%ct origin/main..HEAD":            {Out: unix(branchStart)},
			},
			func(g model.Git) bool { return g.OldestUnpushed.Equal(branchStart) && g.BranchStart.Equal(branchStart) },
		},
		{
			"on the default branch nothing is a branch of its own",
			map[string]platformtest.Result{
				at + "branch --show-current":                         {Out: "main\n"},
				at + "symbolic-ref --short refs/remotes/origin/HEAD": {Out: "origin/main\n"},
				at + "rev-parse --abbrev-ref @{upstream}":            {Out: "origin/main\n"},
			},
			func(g model.Git) bool {
				return g.BranchStart.IsZero() && g.OldestUnpushed.IsZero() && g.MergedBranches == 0
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Cmds = tt.cmds
			got, err := vcs.Git{Sys: sys}.Git(context.Background(), "/work")
			if err != nil || !tt.want(got) {
				t.Errorf("Git() = %+v, %v", got, err)
			}
		})
	}
}

// remembered is a history read earlier, as the cache answers it.
type remembered struct{ history model.History }

func (r remembered) History(context.Context, string, string) (model.History, error) {
	return r.history, nil
}

func TestGitTakesTheHistoryItIsGiven(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Cmds["git --no-optional-locks -C /work branch --show-current"] = platformtest.Result{Out: "feature/x\n"}
	got, err := vcs.Git{Sys: sys, History: remembered{history: model.History{CommitsToday: 7, Streak: 3, DefaultBranch: "main"}}}.Git(context.Background(), "/work")
	if err != nil || got.CommitsToday != 7 || got.Streak != 3 || got.DefaultBranch != "main" {
		t.Errorf("Git() = %+v, %v; the history given is used, not read again", got, err)
	}
}

func TestReviewQueue(t *testing.T) {
	t.Parallel()
	const command = "gh pr list --search review-requested:@me --json createdAt"
	tests := []struct {
		name string
		out  platformtest.Result
		want model.ReviewQueue
		err  error
	}{
		{
			"two waiting, the oldest found",
			platformtest.Result{Out: `[{"createdAt":"2026-10-03T03:00:00Z"},{"createdAt":"2026-10-02T03:00:00Z"},{"createdAt":"x"}]`},
			model.ReviewQueue{Count: 3, Oldest: time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC)},
			nil,
		},
		{"none waiting", platformtest.Result{Out: `[]`}, model.ReviewQueue{}, repository.ErrNone},
		{"gh failed", platformtest.Result{Err: errExit}, model.ReviewQueue{}, repository.ErrNone},
		{"not JSON", platformtest.Result{Out: "x"}, model.ReviewQueue{}, repository.ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Cmds[command] = tt.out
			got, err := vcs.GitHub{Sys: sys}.ReviewQueue(context.Background(), "/work")
			if got.Count != tt.want.Count || !got.Oldest.Equal(tt.want.Oldest) || !errors.Is(err, tt.err) {
				t.Errorf("ReviewQueue() = %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestGitHub(t *testing.T) {
	t.Parallel()
	const command = "gh pr view --json number,reviewDecision,statusCheckRollup,additions,deletions,changedFiles,createdAt,isDraft,mergeable,assignees,reviewRequests,latestReviews"
	tests := []struct {
		name string
		out  platformtest.Result
		want model.PullRequest
		err  error
	}{
		{
			"checks of every kind",
			platformtest.Result{Out: `{"number":2996,"reviewDecision":"CHANGES_REQUESTED","statusCheckRollup":[
				{"conclusion":"SUCCESS"},{"conclusion":"neutral"},{"conclusion":"SKIPPED"},{"state":"SUCCESS"},
				{"conclusion":"FAILURE"},{"conclusion":"CANCELLED"},{"conclusion":"TIMED_OUT"},{"conclusion":"ACTION_REQUIRED"},{"state":"ERROR"},{"state":"FAILURE"},
				{"conclusion":"","state":"PENDING"},{"status":"IN_PROGRESS"}]}`},
			model.PullRequest{Number: 2996, Passed: 4, Failed: 6, Pending: 2, Review: model.ReviewChangesRequested},
			nil,
		},
		{"no checks and no review", platformtest.Result{Out: `{"number":7,"reviewDecision":"","statusCheckRollup":[]}`}, model.PullRequest{Number: 7}, nil},
		{
			"the people: assignees, reviews requested and given, none of them still known",
			platformtest.Result{Out: `{"number":7,"assignees":[{"login":"a"}],"reviewRequests":[],"latestReviews":[{"state":"APPROVED"},{"state":"COMMENTED"}]}`},
			model.PullRequest{Number: 7, Assignees: 1, Reviews: 2, PeopleKnown: true},
			nil,
		},
		{
			"the size, the age, a draft with conflicts",
			platformtest.Result{Out: `{"number":7,"additions":350,"deletions":120,"changedFiles":9,"createdAt":"2026-10-01T04:09:05Z","isDraft":true,"mergeable":"CONFLICTING"}`},
			model.PullRequest{Number: 7, Additions: 350, Deletions: 120, Files: 9, Created: time.Date(2026, 10, 1, 4, 9, 5, 0, time.UTC), Draft: true, Conflicts: true},
			nil,
		},
		{
			"members of an unexpected type are skipped, the rest is kept",
			platformtest.Result{Out: `{"number":7,"reviewDecision":null,"statusCheckRollup":"none"}`},
			model.PullRequest{Number: 7},
			nil,
		},
		{"no pull request for the branch", platformtest.Result{Err: errExit}, model.PullRequest{}, repository.ErrNone},
		{"output that is not JSON", platformtest.Result{Out: "no pull requests found"}, model.PullRequest{}, repository.ErrNone},
		{"no number", platformtest.Result{Out: `{"reviewDecision":"APPROVED"}`}, model.PullRequest{}, repository.ErrNone},
		{"a number of the wrong type", platformtest.Result{Out: `{"number":"7"}`}, model.PullRequest{}, repository.ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Cmds[command] = tt.out
			got, err := vcs.GitHub{Sys: sys}.PullRequest(context.Background(), "/work", "develop")
			if !got.Created.Equal(tt.want.Created) {
				t.Errorf("Created = %v, want %v", got.Created, tt.want.Created)
			}
			got.Created, tt.want.Created = time.Time{}, time.Time{}
			if !reflect.DeepEqual(got, tt.want) || !errors.Is(err, tt.err) {
				t.Errorf("PullRequest() = %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

// A git that is killed while it holds index.lock leaves the lock behind, and
// the user's next commit fails. No git the status line starts may take it.
func TestGitNeverTakesTheIndexLock(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Cmds["git --no-optional-locks -C /work branch --show-current"] = platformtest.Result{Out: "develop\n"}
	if _, err := (vcs.Git{Sys: sys}).Git(t.Context(), "/work"); err != nil {
		t.Fatal(err)
	}
	calls := sys.Calls()
	if len(calls) == 0 {
		t.Fatal("git was not run")
	}
	for _, call := range calls {
		if !strings.HasPrefix(call, "git --no-optional-locks ") {
			t.Errorf("%q may take index.lock", call)
		}
	}
}
