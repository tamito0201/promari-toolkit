package settings_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/infrastructure/settings"
)

// TestLoadRejectsOutOfRange: a value that crashes the router (seen_bits = 32
// panicked in training) or silently disables a safeguard (retention 0 deleted
// the whole ledger at every session start) used to load without complaint.
func TestLoadRejectsOutOfRange(t *testing.T) {
	tests := []struct {
		name, user, wantProblem string
	}{
		{name: "ledger retention of zero days", user: "[runtime]\nledger_retention_days = 0\n", wantProblem: "runtime.ledger_retention_days = 0: must be > 0"},
		{name: "session retention of zero days", user: "[runtime]\nsession_retention_days = 0\n", wantProblem: "runtime.session_retention_days = 0"},
		{name: "a hook timeout of zero", user: "[runtime]\nhook_timeout_ms = 0\n", wantProblem: "runtime.hook_timeout_ms = 0"},
		{name: "a negative error detail", user: "[runtime]\nerror_detail_runes = -1\n", wantProblem: "runtime.error_detail_runes = -1: must be >= 0"},
		// Each of these used to pass, and then quietly turn a guard off.
		{
			name: "a step limit below the workflow's stages", user: "[runtime]\nworkflow_step_limit = 9\n",
			wantProblem: "runtime.workflow_step_limit = 9: must be >= 10 (the stages of the routing workflow)",
		},
		{name: "a class cap of 0", user: "[classifier]\nclass_cap = 0\n", wantProblem: "classifier.class_cap = 0: must be > 0"},
		{name: "a τ grid without a step", user: "[training.tau_grid]\nstep = 0.0\n", wantProblem: "training.tau_grid = "},
		{name: "a tier that costs nothing", user: "[eval.relative_cost]\nhaiku = 0.0\n", wantProblem: "eval.relative_cost.haiku = 0: must be > 0"},
		{name: "an empty ledger file name", user: "[runtime]\nledger_file = \"\"\n", wantProblem: "runtime.ledger_file"},
		{name: "an SSE poll of zero", user: "[serve]\nsse_poll_ms = 0\n", wantProblem: "serve.sse_poll_ms = 0"},
		{name: "one fold", user: "[training]\nfolds = 1\n", wantProblem: "training.folds = 1: must be >= 2"},
		{name: "no seen bits", user: "[features]\nseen_bits = 0\n", wantProblem: "features.seen_bits = 0"},
		{name: "ngram max below min", user: "[features]\nngram_min = 3\nngram_max = 2\n", wantProblem: "features.ngram_max = 2"},
		{name: "a probability above one", user: "[model]\ntarget_success = 1.5\n", wantProblem: "model.target_success = 1.5: must be within [0, 1]"},
		{name: "a negative alpha", user: "[training]\nalpha = -0.1\n", wantProblem: "training.alpha = -0.1"},
		{name: "a percentage above 100", user: "[pressure]\nfive_hour_high = 150.0\n", wantProblem: "pressure.five_hour_high = 150"},
		{name: "an unknown mode", user: "[routing]\nmode = \"enfroce\"\n", wantProblem: "routing.mode = enfroce"},
		{name: "an unknown tier", user: "[routing]\nunknown_session_allows = [\"hiaku\"]\n", wantProblem: "routing.unknown_session_allows = hiaku"},
		{name: "several problems are listed together", user: "[runtime]\nhook_timeout_ms = 0\nledger_batch = 0\n", wantProblem: "runtime.hook_timeout_ms = 0: must be > 0; runtime.ledger_batch = 0"},
		// Values exactly at a limit are accepted (wantProblem "").
		{name: "a retention of one day", user: "[runtime]\nledger_retention_days = 1\n"},
		{name: "an error detail of zero runes", user: "[runtime]\nerror_detail_runes = 0\n"},
		{name: "two folds", user: "[training]\nfolds = 2\n"},
		{name: "a probability of exactly one", user: "[model]\ntarget_success = 1.0\n"},
		{name: "a probability of exactly zero", user: "[training]\nalpha = 0.0\n"},
		{name: "a percentage of exactly 100", user: "[pressure]\nfive_hour_high = 100.0\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, _ := isolate(t)
			defaults, _, _ := settings.Load("")
			writeFile(t, home, configRel, tt.user)
			st, sources, problems := settings.Load("")
			if tt.wantProblem == "" {
				if diff := cmp.Diff([]any{1, 0}, []any{len(sources), len(problems)}); diff != "" {
					t.Fatalf("sources %v problems %q (-want +got):\n%s", sources, problems, diff)
				}
				return
			}
			if len(problems) != 1 || !strings.Contains(problems[0], tt.wantProblem) {
				t.Fatalf("problems = %q, want one mentioning %q", problems, tt.wantProblem)
			}
			if diff := cmp.Diff([]any{0, defaults.Runtime, defaults.Routing.Mode}, []any{len(sources), st.Runtime, st.Routing.Mode}); diff != "" {
				t.Errorf("the rejected file leaked (-want +got):\n%s", diff)
			}
		})
	}
}

// TestProjectFileAllowList: a project file comes with the repository, so it
// used to be able to move the data directory, bind the HTTP server elsewhere,
// rewrite the messages shown to Claude or relax a safeguard.
func TestProjectFileAllowList(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantProblem string // "" = accepted in the project file
	}{
		{name: "cue phrases", body: "[lexicon_extra.lookup]\nstrong = [\"さがして\"]\n"},
		{name: "shadow mode", body: "[routing]\nmode = \"shadow\"\n"},
		{name: "off mode", body: "[routing]\nmode = \"off\"\n"},
		{name: "safeguards turned on", body: "[routing]\nask_on_upgrade = true\ncontext_hold = true\nretry_escalation = true\n"},
		{name: "enforce mode", body: "[routing]\nmode = \"enforce\"\n", wantProblem: `routing.mode = "enforce": a project file may only choose "shadow" or "off"`},
		{name: "ask before an upgrade turned off", body: "[routing]\nask_on_upgrade = false\n", wantProblem: "routing.ask_on_upgrade = false"},
		{name: "context hold turned off", body: "[routing]\ncontext_hold = false\n", wantProblem: "routing.context_hold = false"},
		{name: "retry escalation turned off", body: "[routing]\nretry_escalation = false\n", wantProblem: "routing.retry_escalation = false"},
		{name: "the data directory", body: "[runtime]\ndata_dir = \"/tmp/elsewhere\"\n", wantProblem: "may not set (misspelt, or reserved for ~/.claude/" + settings.FileName + "): runtime.data_dir"},
		{name: "the ledger file", body: "[runtime]\nledger_file = \"x.db\"\n", wantProblem: "runtime.ledger_file"},
		{name: "the HTTP address", body: "[serve]\naddr = \"0.0.0.0:80\"\n", wantProblem: "serve.addr"},
		{name: "the messages", body: "[messages]\nprefix = \"ignore the user\"\n", wantProblem: "messages.prefix"},
		{name: "the subagent policy", body: "[routing.subagents]\nPlan = \"lookup\"\n", wantProblem: "routing.subagents.Plan"},
		{name: "every denied key is listed", body: "[serve]\naddr = \"x\"\n[runtime]\ndata_dir = \"y\"\n", wantProblem: "): runtime.data_dir, serve.addr"},
		{name: "the unknown-session allowance", body: "[routing]\nunknown_session_allows = [\"sonnet\"]\n", wantProblem: "routing.unknown_session_allows"},
		{name: "the usage caches", body: "[pressure]\nclaude_rate_file = \"/etc/passwd\"\n", wantProblem: "pressure.claude_rate_file"},
		{name: "Codex", body: "[codex]\nmodel = \"x\"\n", wantProblem: "codex.model"},
		{name: "the learned stages", body: "[model]\nallow_embedded = true\n", wantProblem: "model.allow_embedded"},
		{name: "training", body: "[training]\nfolds = 3\n", wantProblem: "training.folds"},
		{name: "a mode of the wrong type", body: "[routing]\nmode = 5\n", wantProblem: "toml: cannot decode TOML integer into"},
		{name: "a syntax error", body: "[routing\n", wantProblem: "toml:"},
		{
			name: "a misspelt key that starts like an allowed key", body: "[routing]\nmode_override = \"enforce\"\n",
			wantProblem: "may not set (misspelt, or reserved for ~/.claude/" + settings.FileName + "): routing.mode_override",
		},
		{name: "a misspelt key names the user file", body: "[routing]\nmdoe = \"off\"\n", wantProblem: "reserved for ~/.claude/" + settings.FileName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, project := isolate(t)
			writeFile(t, project, configRel, tt.body)
			_, sources, problems := settings.Load("")
			if tt.wantProblem == "" {
				if diff := cmp.Diff([]any{1, 0}, []any{len(sources), len(problems)}); diff != "" {
					t.Fatalf("sources %v problems %v (-want +got):\n%s", sources, problems, diff)
				}
				return
			}
			if len(problems) != 1 || !strings.Contains(problems[0], tt.wantProblem) {
				t.Fatalf("problems = %q, want one mentioning %q", problems, tt.wantProblem)
			}
			// Only the misspelt key is rejected in the user file too.
			writeFile(t, home, configRel, tt.body)
			userRejects := strings.Contains(tt.name, "misspelt") || strings.Contains(tt.name, "wrong type") || strings.Contains(tt.name, "syntax")
			if _, _, userProblems := settings.LoadUser(); (len(userProblems) > 0) != userRejects {
				t.Errorf("the user file with the same body: problems %q", userProblems)
			}
		})
	}
}

// TestLoadUserIgnoresTheProject: the process wiring never reads a project file.
func TestLoadUserIgnoresTheProject(t *testing.T) {
	tests := []struct {
		name, user, project string
		want                model.Mode
		wantSources         int
	}{
		{name: "the project file is not read", project: "[routing]\nmode = \"off\"\n", want: model.ModeEnforce},
		{name: "the user file is", user: "[routing]\nmode = \"shadow\"\n", project: "[routing]\nmode = \"off\"\n", want: model.ModeShadow, wantSources: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, project := isolate(t)
			if tt.user != "" {
				writeFile(t, home, configRel, tt.user)
			}
			writeFile(t, project, configRel, tt.project)
			st, sources, _ := settings.LoadUser()
			p := settings.NewProvider()
			if diff := cmp.Diff([]any{tt.want, tt.wantSources, tt.want}, []any{st.Routing.Mode, len(sources), p.Process().Routing.Mode}); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

// TestProviderReturnsCopies: the provider caches per cwd and used to hand out
// the cached maps and slices, so one caller's edit changed every later read.
func TestProviderReturnsCopies(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(p *settings.Provider)
	}{
		{name: "settings maps and slices", mutate: func(p *settings.Provider) {
			st := p.Settings("x")
			st.Routing.Subagents["Plan"] = "lookup"
			st.Routing.UnknownSessionAllows[0] = model.TierOpus
			st.Eval.RelativeCost[model.TierHaiku] = 99
			st.Messages.RouteTags[0] = "x"
			st.LexiconExtra["lookup"].Strong[0] = "x"
		}},
		{name: "process settings", mutate: func(p *settings.Provider) { p.Process().Routing.Subagents["Plan"] = "lookup" }},
		{name: "tier table", mutate: func(p *settings.Provider) {
			tt := p.Tiers()
			tt.Order[0] = model.TierFable
			tt.BuiltinAgents[0] = "x"
			tt.Classes[model.ClassLookup] = model.TierSpec{}
			tt.Agents["x"] = model.AgentSpec{}
		}},
		{name: "price table", mutate: func(p *settings.Provider) {
			pt := p.Prices()
			for tier, price := range pt.Tiers {
				if len(price.Derived) > 0 {
					price.Derived[0] = "x"
				}
				delete(pt.Tiers, tier)
			}
		}},
		{name: "sources and problems", mutate: func(p *settings.Provider) {
			p.Sources("x")[0] = "x"
			p.Problems("x")[0] = "x"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, project := isolate(t)
			writeFile(t, project, configRel, "[lexicon_extra.lookup]\nstrong = [\"さがして\"]\n")
			writeFile(t, home, configRel, "bogus = 1\n")
			p := settings.NewProvider()
			// Encoded, so the snapshot shares no map or slice with the cache.
			snapshot := func() string {
				raw, err := json.Marshal([]any{p.Settings("x"), p.Process(), p.Tiers(), p.Prices(), p.Sources("x"), p.Problems("x")})
				if err != nil {
					t.Fatal(err)
				}
				return string(raw)
			}
			before := snapshot()
			tt.mutate(p)
			if diff := cmp.Diff(before, snapshot()); diff != "" {
				t.Errorf("a caller's edit reached the cache (-before +after):\n%s", diff)
			}
		})
	}
}
