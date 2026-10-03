package usecase_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
	"promari-model-router/pkg/fp"
)

func TestDoctor(t *testing.T) {
	spec := model.FeatureSpec{HashBuckets: 8}
	trusted := model.Artifact{
		Origin: model.OriginLocal, Source: "mine", Samples: 42, TrainedAt: testNow,
		Classes: []model.Class{model.ClassLookup}, Weights: [][]float64{make([]float64, spec.Dim())}, Bias: []float64{0}, Features: spec,
	}
	failure := func(ago time.Duration, msg string) *model.Failure {
		return &model.Failure{At: testNow.Add(-ago), Message: msg}
	}
	embedded := trusted
	embedded.Origin = ""
	badTiers := model.TierTable{Agents: map[string]model.AgentSpec{"absent": {Model: model.TierHaiku}}}
	week := "(7 days)"
	type check = usecase.Check
	tests := []struct {
		name  string
		setup func(f *fixture)
		// want lists the checks by name; a Detail of "*" is not compared.
		want []check
	}{
		{
			name: "healthy",
			setup: func(f *fixture) {
				f.deps.Resolver.Settings = fakeSettings{file: "claude-opus-5-5"}
				f.deps.Usage = fakeUsage{claude: model.Pressure{Known: true, FiveHour: fp.Some(10.0), SevenDay: fp.Some(20.0)}, codex: model.CodexQuota{Available: true, Used: fp.Some(5.0)}}
				f.artifacts.art = trusted
				f.ledger.checked = 3
				f.ledger.entries = []model.Entry{{At: testNow, Event: model.EventPrompt}}
			},
			want: []check{
				{"ok", "mode", "enforce"},
				{"ok", "data dir", "/data"},
				{"ok", "ledger", "/data/ledger.db"},
				{"ok", "session model fallback", "claude-opus-5-5 via settings (uncertain sources never cap a route)"},
				{"ok", "Claude usage cache", "known=true high=false 5h=10 7d=20"},
				{"ok", "Codex usage", "available=true used=5"},
				{"ok", "artifact", "local, mine, 42 samples, trained 2026-09-30"},
				{"ok", "agent tiers", "consistent with data/tiers.toml"},
				{"ok", "ledger chain", "3 rows intact"},
				{"ok", "ledger " + week, "1 entries"},
				{"ok", "hook errors " + week, "0"},
			},
		},
		{
			name: "degraded everywhere",
			setup: func(f *fixture) {
				f.config.mutate = func(s *model.Settings) { s.Routing.Mode = "bogus"; s.Doctor.WindowDays = 7 }
				f.config.problems = []string{"/x.toml: unknown key"}
				f.config.sources = []string{"/y.toml"}
				f.config.tiers = &badTiers
				f.deps.Env = fakeEnv{model.ForceEnv: "1", "CLAUDE_CODE_SUBAGENT_MODEL": "v", "CLAUDE_CODE_EFFORT_LEVEL": "v", usecase.LauncherSourceEnv: "dist"}
				f.deps.Usage = fakeUsage{}
				f.artifacts.loadErr = errArtifact
				f.ledger.verifyErr = errLedger
			},
			want: []check{
				{"fail", "mode", "bogus"},
				{"fail", "config file rejected", "/x.toml: unknown key"},
				{"ok", "data dir", "/data"},
				{"ok", "ledger", "/data/ledger.db"},
				{"ok", "config file", "/y.toml"},
				{"warn", "launcher", "running the development build in dist/ (may lag behind the source)"},
				{"fail", "CLAUDE_CODE_SUBAGENT_MODEL_FORCE", "set: routing is disabled"},
				{"warn", "CLAUDE_CODE_SUBAGENT_MODEL", "v (a default only; routed models still win)"},
				{"warn", "CLAUDE_CODE_EFFORT_LEVEL", "v: overrides the effort in agent frontmatter"},
				{"warn", "session model fallback", " via unknown (uncertain sources never cap a route)"},
				{"warn", "Claude usage cache", "known=false high=false 5h=-1 7d=-1"},
				{"warn", "Codex usage", "available=false used=-1"},
				{"warn", "artifact", "no trained artifact: rules only"},
				{"fail", "agent tiers", "agents/absent.md is missing"},
				{"fail", "ledger chain", "ledger down"},
				{"warn", "ledger " + week, "0 entries"},
				{"ok", "hook errors " + week, "0"},
			},
		},
		{
			name: "untrusted artifact, broken chain, recent hook errors",
			setup: func(f *fixture) {
				f.artifacts.art = embedded
				f.ledger.broken = 7
				f.ledger.entries = []model.Entry{
					{At: testNow, Event: model.EventError, Reason: "a", Detail: "first"},
					{At: testNow, Event: model.EventError, Reason: "pre-tool-use", Detail: "last"},
				}
			},
			want: []check{
				{"ok", "mode", "enforce"},
				{"ok", "data dir", "/data"},
				{"ok", "ledger", "/data/ledger.db"},
				{"warn", "session model fallback", "*"},
				{"ok", "Claude usage cache", "*"},
				{"ok", "Codex usage", "*"},
				{"warn", "artifact", "embedded artifact only (mine, 42 samples): not used for routing; rules only"},
				{"ok", "agent tiers", "consistent with data/tiers.toml"},
				{"fail", "ledger chain", "broken at row 7"},
				{"ok", "ledger " + week, "2 entries"},
				{"fail", "hook errors " + week, "2; last: pre-tool-use last"},
			},
		},
		{
			// A broken local artifact used to read as "no trained artifact".
			name: "a broken local artifact, an installed launcher, a model name in FORCE",
			setup: func(f *fixture) {
				f.deps.Env = fakeEnv{model.ForceEnv: "sonnet", usecase.LauncherSourceEnv: "installed"}
				f.artifacts.loadErr = fmt.Errorf("%w: /data/artifact.json: no classes", model.ErrArtifactBroken)
			},
			want: []check{
				{"ok", "mode", "enforce"},
				{"ok", "data dir", "/data"},
				{"ok", "ledger", "/data/ledger.db"},
				{"ok", "launcher", "installed"},
				{"warn", "session model fallback", "*"},
				{"ok", "Claude usage cache", "*"},
				{"ok", "Codex usage", "*"},
				{"fail", "artifact", "local artifact is broken: /data/artifact.json: no classes; routing runs on rules only"},
				{"ok", "agent tiers", "*"},
				{"ok", "ledger chain", "*"},
				{"warn", "ledger " + week, "0 entries"},
				{"ok", "hook errors " + week, "0"},
			},
		},
		{
			// "hook errors 0" used to be reported whatever happened to the read.
			name: "an unreadable ledger and a failure the ledger never saw",
			setup: func(f *fixture) {
				f.ledger.sinceErr = errLedger
				f.failures.last = failure(time.Hour, "PreToolUse: boom")
				f.launcher.last = failure(time.Minute, "checksum mismatch")
			},
			want: []check{
				{"ok", "mode", "enforce"},
				{"ok", "data dir", "/data"},
				{"ok", "ledger", "/data/ledger.db"},
				{"warn", "session model fallback", "*"},
				{"ok", "Claude usage cache", "*"},
				{"ok", "Codex usage", "*"},
				{"warn", "artifact", "*"},
				{"ok", "agent tiers", "*"},
				{"ok", "ledger chain", "*"},
				{"fail", "ledger " + week, "unreadable: ledger down"},
				{"fail", "hook failure outside the ledger", "2026-09-30T11:00:00Z: PreToolUse: boom"},
				{"fail", "launcher failure", "2026-09-30T11:59:00Z: checksum mismatch"},
			},
		},
		{
			name: "a failure the ledger has recovered from warns; one older than the window is not shown",
			setup: func(f *fixture) {
				f.ledger.entries = []model.Entry{{At: testNow, Event: model.EventPrompt}}
				f.failures.last = failure(time.Hour, "PostToolUse: boom")
				f.launcher.last = failure(8*24*time.Hour, "old")
			},
			want: []check{
				{"ok", "mode", "enforce"},
				{"ok", "data dir", "/data"},
				{"ok", "ledger", "/data/ledger.db"},
				{"warn", "session model fallback", "*"},
				{"ok", "Claude usage cache", "*"},
				{"ok", "Codex usage", "*"},
				{"warn", "artifact", "*"},
				{"ok", "agent tiers", "*"},
				{"ok", "ledger chain", "*"},
				{"ok", "ledger " + week, "1 entries"},
				{"ok", "hook errors " + week, "0"},
				{"warn", "hook failure outside the ledger", "2026-09-30T11:00:00Z (the ledger has recorded entries since): PostToolUse: boom"},
			},
		},
		{
			name:  "an unreadable failure file fails",
			setup: func(f *fixture) { f.launcher.readErr = errors.New("permission denied") },
			want: []check{
				{"ok", "mode", "enforce"},
				{"ok", "data dir", "/data"},
				{"ok", "ledger", "/data/ledger.db"},
				{"warn", "session model fallback", "*"},
				{"ok", "Claude usage cache", "*"},
				{"ok", "Codex usage", "*"},
				{"warn", "artifact", "*"},
				{"ok", "agent tiers", "*"},
				{"ok", "ledger chain", "*"},
				{"warn", "ledger " + week, "0 entries"},
				{"ok", "hook errors " + week, "0"},
				{"fail", "launcher failure", "unreadable: permission denied"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, func(f *fixture) {
				f.config.mutate = func(s *model.Settings) { s.Doctor.WindowDays = 7 }
				f.deps.Usage = fakeUsage{claude: model.Pressure{Known: true}, codex: model.CodexQuota{Available: true}}
				tt.setup(f)
			})
			got := f.doctor().Execute(t.Context(), "")
			if len(got) == len(tt.want) {
				for i := range got {
					if tt.want[i].Detail == "*" {
						got[i].Detail = "*"
					}
				}
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("checks (-want +got):\n%s", diff)
			}
		})
	}
}
