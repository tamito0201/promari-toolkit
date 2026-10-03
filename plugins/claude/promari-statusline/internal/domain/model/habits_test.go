package model_test

import (
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

func TestHabitJudgements(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{
		"feature/login-ui": true, "fix/cart-bug": true, "docs/statusline-guide": true, "feat/a/b-c": true,
		"login": false, "test": false, "fix/fix": false, "feature/wip": false, "feature/ab": false, "hoge/login": false,
	} {
		if got := model.GoodBranchName(name); got != want {
			t.Errorf("GoodBranchName(%q) = %v", name, got)
		}
	}
	if !model.IsDefaultBranch("develop", "develop") || model.IsDefaultBranch("main", "develop") || !model.IsDefaultBranch("master", "") || model.IsDefaultBranch("develop", "") {
		t.Error("IsDefaultBranch")
	}
	for subject, want := range map[string]bool{
		"fix": true, "update": true, "修正": true, "aaa": true, "feat: a": true, "fix(x): 修正": true, ".": true, "wip": true,
		"feat: ログイン画面にメール形式のバリデーションを追加": false, "Fix the cart total when a coupon is removed": false, `Revert "feat: a"`: false,
	} {
		if got := model.IsVagueCommit(subject); got != want {
			t.Errorf("IsVagueCommit(%q) = %v", subject, got)
		}
	}
	for subject, want := range map[string]bool{"feat: a": true, "fix(ui)!: b": true, "docs: c": true, "Feat: a": false, "feature: a": false, "fix:no space": false} {
		if got := model.IsConventionalCommit(subject); got != want {
			t.Errorf("IsConventionalCommit(%q) = %v", subject, got)
		}
	}
	if !model.MarksConflict("<<<<<<< HEAD") || !model.MarksConflict("=======") || !model.MarksConflict(">>>>>>> feature/x") || model.MarksConflict("==== title") || model.MarksConflict("a <<<<<<< b") {
		t.Error("MarksConflict")
	}
	for path, want := range map[string]bool{
		"obj/": true, "src/App/bin/Debug/a.dll": true, ".vs/": true, "node_modules/x": true, ".DS_Store": true, "a/.env": true, ".env.local": true,
		"App.csproj.user": true, "debug.log": true, `"a b/obj/x"`: true, "src/binary.go": false, "objects/a.go": false, ".envrc": false, "README.md": false,
	} {
		if got := model.IsJunkPath(path); got != want {
			t.Errorf("IsJunkPath(%q) = %v", path, got)
		}
	}
}

func TestCommitMeasures(t *testing.T) {
	t.Parallel()
	g := model.Git{CommitsToday: 4, TodayAdded: 120, TodayDeleted: 20}
	if size, ok := g.CommitSize(); !ok || size != 35 {
		t.Errorf("CommitSize = %v, %v", size, ok)
	}
	if ratio, ok := g.AddDeleteRatio(); !ok || ratio != 6 {
		t.Errorf("AddDeleteRatio = %v, %v", ratio, ok)
	}
	if _, ok := (model.Git{CommitsToday: 2}).CommitSize(); ok {
		t.Error("commits without lines have no size")
	}
	if _, ok := (model.Git{TodayAdded: 3}).AddDeleteRatio(); ok {
		t.Error("nothing deleted, no ratio")
	}

	today := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name             string
		days             []string
		current, longest int
	}{
		{"up to today", []string{"2026-10-03", "2026-10-02", "2026-10-01", "2026-09-20", "2026-09-19"}, 3, 3},
		{"up to yesterday, today not yet", []string{"2026-10-02", "2026-10-01"}, 2, 2},
		{"broken two days ago", []string{"2026-09-30", "2026-09-29", "2026-09-28", "2026-09-27"}, 0, 4},
		{"duplicates and garbage", []string{"2026-10-03", "2026-10-03", "x"}, 1, 1},
		{"nothing", nil, 0, 0},
	}
	for _, tt := range tests {
		if current, longest := model.Streaks(tt.days, today); current != tt.current || longest != tt.longest {
			t.Errorf("%s: Streaks = %d, %d; want %d, %d", tt.name, current, longest, tt.current, tt.longest)
		}
	}
}
