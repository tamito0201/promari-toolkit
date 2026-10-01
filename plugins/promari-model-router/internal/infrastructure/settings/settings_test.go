package settings_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/settings"
)

// writeFile creates dir/rel (and its parents) with body.
func writeFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// isolate points HOME and CLAUDE_PROJECT_DIR at fresh directories.
func isolate(t *testing.T) (home, project string) {
	t.Helper()
	home, project = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_PROJECT_DIR", project)
	return home, project
}

// panicMessage runs f and returns the recovered panic as a string ("" = none).
func panicMessage(f func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprint(r)
		}
	}()
	f()
	return ""
}

var configRel = filepath.Join(".claude", settings.FileName)

func TestLoadLayers(t *testing.T) {
	tests := []struct {
		name         string
		user         string
		project      string
		projectIsDir bool
		wantMode     model.Mode
		wantMargin   int
		wantSources  int
		wantProblems []string // substrings
	}{
		{name: "defaults only", wantMode: model.ModeEnforce, wantMargin: 2},
		{
			name: "user overrides one key, others keep defaults",
			user: "[classifier]\nmin_margin = 4\n", wantMode: model.ModeEnforce, wantMargin: 4, wantSources: 1,
		},
		{
			name: "project wins over user", user: "[routing]\nmode = \"shadow\"\n", project: "[routing]\nmode = \"off\"\n",
			wantMode: model.ModeOff, wantMargin: 2, wantSources: 2,
		},
		{
			name: "unknown key rejects the whole file", project: "[classifier]\nmin_margn = 9\nmin_confidence = 99\n",
			wantMode: model.ModeEnforce, wantMargin: 2, wantProblems: []string{"min_margn"},
		},
		{
			name: "misspelt lexicon_extra class rejects the file", project: "[routing]\nmode = \"off\"\n[lexicon_extra.mechanicl]\nstrong = [\"x\"]\n",
			wantMode: model.ModeEnforce, wantMargin: 2, wantProblems: []string{"lexicon_extra.mechanicl"},
		},
		{
			name: "items on a class rejects the file", user: "[lexicon_extra.lookup]\nitems = [\"x\"]\n",
			wantMode: model.ModeEnforce, wantMargin: 2, wantProblems: []string{"lexicon_extra.lookup"},
		},
		{
			name: "strong on a flag list rejects the file", user: "[lexicon_extra.danger]\nstrong = [\"x\"]\n",
			wantMode: model.ModeEnforce, wantMargin: 2, wantProblems: []string{"lexicon_extra.danger"},
		},
		{
			name: "syntax error rejects the file", user: "[routing\nmode = shadow", wantMode: model.ModeEnforce, wantMargin: 2,
			wantProblems: []string{"promari-model-router.toml"},
		},
		{
			name: "an unreadable file (a directory) is a problem, not a crash", projectIsDir: true,
			wantMode: model.ModeEnforce, wantMargin: 2, wantProblems: []string{"is a directory"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, project := isolate(t)
			if tt.user != "" {
				writeFile(t, home, configRel, tt.user)
			}
			if tt.project != "" {
				writeFile(t, project, configRel, tt.project)
			}
			if tt.projectIsDir {
				if err := os.MkdirAll(filepath.Join(project, configRel), 0o700); err != nil {
					t.Fatal(err)
				}
			}

			st, sources, problems := settings.Load("")
			type summary struct {
				Mode    model.Mode
				Margin  int
				Sources int
			}
			if diff := cmp.Diff(summary{tt.wantMode, tt.wantMargin, tt.wantSources}, summary{st.Routing.Mode, st.Classifier.MinMargin, len(sources)}); diff != "" {
				t.Errorf("Load() mismatch (-want +got):\n%s", diff)
			}
			if len(problems) != len(tt.wantProblems) {
				t.Fatalf("problems = %v", problems)
			}
			for i, want := range tt.wantProblems {
				if !strings.Contains(problems[i], want) {
					t.Errorf("problem %q does not mention %q", problems[i], want)
				}
			}
			// A rejected file must not leak partial values into the defaults.
			if st.Classifier.MinConfidence == 99 {
				t.Error("values from a rejected file leaked")
			}
		})
	}
}

func TestLoadBrokenDefaults(t *testing.T) {
	tests := []struct {
		name      string
		data      fstest.MapFS
		wantPanic string
	}{
		{name: "missing defaults is a build defect", data: fstest.MapFS{}, wantPanic: "embedded defaults.toml is missing"},
		{
			name: "invalid defaults is a build defect", data: fstest.MapFS{"data/defaults.toml": {Data: []byte("nonsense = 1\n")}},
			wantPanic: "embedded defaults.toml is invalid: unknown key(s): nonsense (line 1, col 1)",
		},
		{
			name: "zero settings are out of range", data: fstest.MapFS{"data/defaults.toml": {Data: nil}},
			wantPanic: "embedded defaults.toml is invalid: routing.mode = : must be",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			got := panicMessage(func() { settings.LoadFrom(tt.data, "") })
			if !strings.HasPrefix(got, tt.wantPanic) || (tt.wantPanic == "") != (got == "") {
				t.Errorf("panic = %q, want prefix %q", got, tt.wantPanic)
			}
		})
	}
}

func TestEmbeddedTables(t *testing.T) {
	tiers := settings.Tiers()
	prices := settings.Prices()
	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "tier order", check: func(t *testing.T) {
				t.Helper()
				if diff := cmp.Diff([]model.Tier{model.TierHaiku, model.TierSonnet, model.TierOpus, model.TierFable}, tiers.Order); diff != "" {
					t.Error(diff)
				}
			},
		},
		{
			name: "fable is never an automatic target", check: func(t *testing.T) {
				t.Helper()
				if tiers.Target(model.ClassArchitecture) == model.TierFable {
					t.Error("fable must never be an automatic target")
				}
			},
		},
		{
			name: "every tier has a dated price", check: func(t *testing.T) {
				t.Helper()
				if prices.AsOf == "" || len(prices.Tiers) != len(tiers.Order) {
					t.Errorf("prices = %+v", prices)
				}
			},
		},
		{
			name: "the flag lists of the lexicon are filled", check: func(t *testing.T) {
				t.Helper()
				isolate(t)
				st, _, _ := settings.Load("")
				if lex := settings.Lexicon(st); len(lex.Danger) == 0 || len(lex.Context) == 0 || len(lex.Correction) == 0 || len(lex.Continuation) == 0 {
					t.Error("lexicon lists are empty")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

func TestMustDecode(t *testing.T) {
	tests := []struct {
		name      string
		data      fstest.MapFS
		wantPanic string
	}{
		{name: "valid file decodes", data: fstest.MapFS{"x.toml": {Data: []byte("as_of = \"2026-09-30\"\n")}}},
		{name: "missing file panics", data: fstest.MapFS{}, wantPanic: "embedded x.toml is invalid: open x.toml: file does not exist"},
		{
			name: "unknown key panics", data: fstest.MapFS{"x.toml": {Data: []byte("as_off = 1\n")}},
			wantPanic: "embedded x.toml is invalid: unknown key(s): as_off (line 1, col 1)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p service.PriceTable
			got := panicMessage(func() { settings.MustDecode(tt.data, "x.toml", &p) })
			if diff := cmp.Diff(tt.wantPanic, got); diff != "" {
				t.Errorf("panic mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLexiconExtra(t *testing.T) {
	type lists struct {
		Danger, Continuation, Context, Correction, Strong, Weak bool
	}
	has := func(lex *service.Lexicon, phrase string) lists {
		mech := lex.Classes[model.ClassMechanical]
		return lists{
			slices.Contains(lex.Danger, phrase), slices.Contains(lex.Continuation, phrase), slices.Contains(lex.Context, phrase),
			slices.Contains(lex.Correction, phrase), slices.Contains(mech.Strong, phrase), slices.Contains(mech.Weak, phrase),
		}
	}
	tests := []struct {
		name   string
		config string
		data   fstest.MapFS // nil: the embedded lexicon
		phrase string
		want   lists
	}{
		{name: "danger items", config: "[lexicon_extra.danger]\nitems = [\"給与計算\"]\n", phrase: "給与計算", want: lists{Danger: true}},
		{name: "continuation items", config: "[lexicon_extra.continuation]\nitems = [\"つづけて\"]\n", phrase: "つづけて", want: lists{Continuation: true}},
		{name: "context items", config: "[lexicon_extra.context]\nitems = [\"さっきの件\"]\n", phrase: "さっきの件", want: lists{Context: true}},
		{name: "correction items", config: "[lexicon_extra.correction]\nitems = [\"ちがうよ\"]\n", phrase: "ちがうよ", want: lists{Correction: true}},
		{name: "class strong phrases", config: "[lexicon_extra.mechanical]\nstrong = [\"ぽちっと整える\"]\n", phrase: "ぽちっと整える", want: lists{Strong: true}},
		{name: "class weak phrases", config: "[lexicon_extra.mechanical]\nweak = [\"軽く整える\"]\n", phrase: "軽く整える", want: lists{Weak: true}},
		{
			name: "a lexicon without classes still takes class phrases", config: "[lexicon_extra.mechanical]\nstrong = [\"ぽちっと整える\"]\n",
			data: fstest.MapFS{"data/lexicon.toml": {Data: []byte("danger = [\"rm -rf\"]\n")}}, phrase: "ぽちっと整える", want: lists{Strong: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, project := isolate(t)
			writeFile(t, project, configRel, tt.config)
			st, _, problems := settings.Load("")
			if len(problems) > 0 {
				t.Fatal(problems)
			}
			lex := settings.Lexicon(st)
			if tt.data != nil {
				lex = settings.LexiconFrom(tt.data, st)
			}
			if diff := cmp.Diff(tt.want, has(lex, tt.phrase)); diff != "" {
				t.Errorf("lexicon lists mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLexiconExtraReachesSignals(t *testing.T) {
	tests := []struct {
		name, config, prompt string
		wantMechanical       bool
		wantDanger           bool
	}{
		{
			name:   "extra phrases score and flag the prompt",
			config: "[lexicon_extra.mechanical]\nstrong = [\"ぽちっと整える\"]\n\n[lexicon_extra.danger]\nitems = [\"給与計算\"]\n",
			prompt: "見出しをぽちっと整える。給与計算の画面で", wantMechanical: true, wantDanger: true,
		},
		{name: "without extras the phrases mean nothing", prompt: "ぽちっと整える給与計算"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, project := isolate(t)
			if tt.config != "" {
				writeFile(t, project, configRel, tt.config)
			}
			st, _, _ := settings.Load("")
			sig := settings.Lexicon(st).ExtractSignals(tt.prompt)
			got := []bool{sig.Scores[model.ClassMechanical] >= st.Classifier.StrongWeight, slices.Contains(sig.Danger, "給与計算")}
			if diff := cmp.Diff([]bool{tt.wantMechanical, tt.wantDanger}, got); diff != "" {
				t.Errorf("signals mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPaths(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		projectEnv string
		pluginData string
		cwd        string
		dataDir    string // runtime.data_dir
		call       func(cwd string, st model.Settings) string
		want       string
	}{
		{name: "user path is under HOME", call: func(string, model.Settings) string { return settings.UserPath() }, want: "/h/.claude/" + settings.FileName},
		{name: "CLAUDE_PROJECT_DIR wins over cwd", projectEnv: "/p", cwd: "/c", call: projectPath, want: "/p/.claude/" + settings.FileName},
		{name: "cwd when CLAUDE_PROJECT_DIR is unset", cwd: "/c", call: projectPath, want: "/c/.claude/" + settings.FileName},
		{name: "working directory when both are empty", call: projectPath, want: filepath.Join(wd, ".claude", settings.FileName)},
		{name: "runtime.data_dir wins", dataDir: "~/d", pluginData: "/env", call: dataDir, want: "/h/d"},
		{name: "then CLAUDE_PLUGIN_DATA", pluginData: "/env", call: dataDir, want: "/env"},
		{name: "then the default", call: dataDir, want: "/h/.claude/plugins/data/promari-model-router"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", "/h")
			t.Setenv("CLAUDE_PROJECT_DIR", tt.projectEnv)
			t.Setenv("CLAUDE_PLUGIN_DATA", tt.pluginData)
			var st model.Settings
			st.Runtime.DataDir = tt.dataDir
			if diff := cmp.Diff(tt.want, tt.call(tt.cwd, st)); diff != "" {
				t.Errorf("path mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func projectPath(cwd string, _ model.Settings) string { return settings.ProjectPath(cwd) }
func dataDir(_ string, st model.Settings) string      { return settings.DataDir(st) }

func TestEnvModel(t *testing.T) {
	type result struct {
		Model string
		OK    bool
	}
	tests := []struct {
		name, env string
		want      result
	}{
		{name: "set", env: "claude-sonnet-4-6", want: result{"claude-sonnet-4-6", true}},
		{name: "unset", env: "", want: result{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ANTHROPIC_MODEL", tt.env)
			m, ok := settings.EnvSettings{}.EnvModel()
			if diff := cmp.Diff(tt.want, result{m, ok}); diff != "" {
				t.Errorf("EnvModel() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestProcessEnv(t *testing.T) {
	tests := []struct {
		name, value string
	}{
		{name: "set", value: "haiku"},
		{name: "unset reads empty", value: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PMR_TEST_PROCESS_ENV", tt.value)
			if diff := cmp.Diff(tt.value, settings.ProcessEnv{}.Getenv("PMR_TEST_PROCESS_ENV")); diff != "" {
				t.Errorf("Getenv() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSettingsModel(t *testing.T) {
	type result struct {
		Model string
		OK    bool
	}
	local := filepath.Join(".claude", "settings.local.json")
	project := filepath.Join(".claude", "settings.json")
	user := filepath.Join(".claude", "settings.json")
	tests := []struct {
		name         string
		projectFiles map[string]string
		userFile     string
		want         result
	}{
		{name: "nothing configured", want: result{}},
		{
			name:         "local settings win",
			projectFiles: map[string]string{local: `{"model":"local"}`, project: `{"model":"project"}`}, userFile: `{"model":"user"}`,
			want: result{"local", true},
		},
		{
			name:         "project settings next",
			projectFiles: map[string]string{project: `{"model":"project"}`}, userFile: `{"model":"user"}`,
			want: result{"project", true},
		},
		{
			name:         "broken and model-less files are skipped",
			projectFiles: map[string]string{local: `{`, project: `{"theme":"dark"}`}, userFile: `{"model":"user"}`,
			want: result{"user", true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, proj := isolate(t)
			for rel, body := range tt.projectFiles {
				writeFile(t, proj, rel, body)
			}
			if tt.userFile != "" {
				writeFile(t, home, user, tt.userFile)
			}
			m, ok := settings.EnvSettings{}.SettingsModel("")
			if diff := cmp.Diff(tt.want, result{m, ok}); diff != "" {
				t.Errorf("SettingsModel() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestProvider(t *testing.T) {
	type result struct {
		Mode     model.Mode
		Sources  int
		Problems int
		Lexicon  bool
		Tiers    int
		Prices   bool
	}
	tests := []struct {
		name    string
		project string
		// rewrite changes the project file after the first read (the cache must hide it).
		rewrite string
		want    result
	}{
		{name: "defaults", want: result{Mode: model.ModeEnforce, Lexicon: true, Tiers: 4, Prices: true}},
		{
			name: "a project override is applied", project: "[routing]\nmode = \"shadow\"\n",
			want: result{Mode: model.ModeShadow, Sources: 1, Lexicon: true, Tiers: 4, Prices: true},
		},
		{
			name: "a rejected file is a problem", project: "bogus = 1\n",
			want: result{Mode: model.ModeEnforce, Problems: 1, Lexicon: true, Tiers: 4, Prices: true},
		},
		{
			name: "results are cached per cwd", project: "[routing]\nmode = \"shadow\"\n", rewrite: "[routing]\nmode = \"off\"\n",
			want: result{Mode: model.ModeShadow, Sources: 1, Lexicon: true, Tiers: 4, Prices: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, project := isolate(t)
			if tt.project != "" {
				writeFile(t, project, configRel, tt.project)
			}
			p := settings.NewProvider()
			_ = p.Settings("x")
			if tt.rewrite != "" {
				writeFile(t, project, configRel, tt.rewrite)
			}
			got := result{
				Mode: p.Settings("x").Routing.Mode, Sources: len(p.Sources("x")), Problems: len(p.Problems("x")),
				Lexicon: p.Lexicon("x") != nil, Tiers: len(p.Tiers().Order), Prices: p.Prices().AsOf != "",
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Provider mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
