// Package settings reads the layered TOML configuration and the embedded data
// tables and turns them into domain values. Layers, lowest first:
//
//	data/defaults.toml (embedded)
//	~/.claude/promari-model-router.toml
//	<project>/.claude/promari-model-router.toml
//
// Each layer is decoded on top of the previous one, so a file only needs the
// keys it changes. The two override layers are decoded strictly: an unknown
// key or an out-of-range value rejects the whole file (reported by `pmr
// doctor` and at SessionStart) instead of being silently ignored.
//
// Trust: the project file arrives with whatever repository was cloned, so it
// is read through an allow-list (projectFile): it may add cue phrases and make
// routing more careful, nothing else. Paths, the HTTP address, the messages
// shown to Claude, the subagent policy and every limit come from the defaults
// and the user's own file only.
package settings

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	modelrouter "promari-model-router"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
	"promari-model-router/internal/infrastructure/fsutil"
)

// FileName is the configuration file name in ~/.claude and <project>/.claude.
const FileName = "promari-model-router.toml"

// UserPath is ~/.claude/promari-model-router.toml.
func UserPath() string { return fsutil.ExpandHome("~/.claude/" + FileName) }

// ProjectPath is <project>/.claude/promari-model-router.toml.
// CLAUDE_PROJECT_DIR wins because hooks may run in a subdirectory.
func ProjectPath(cwd string) string {
	dir := cmp.Or(os.Getenv("CLAUDE_PROJECT_DIR"), cwd)
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return filepath.Join(dir, ".claude", FileName)
}

// unknownKeysError names the keys a strict decode did not accept: "fields are
// missing in the target struct" alone does not tell a user which line to fix.
type unknownKeysError struct{ keys []string }

func (e *unknownKeysError) Error() string { return "unknown key(s): " + strings.Join(e.keys, ", ") }

func decode(raw []byte, v any, strict bool) error {
	dec := toml.NewDecoder(bytes.NewReader(raw))
	if strict {
		dec.DisallowUnknownFields()
	}
	err := dec.Decode(v)
	if sm, ok := errors.AsType[*toml.StrictMissingError](err); ok {
		keys := make([]string, 0, len(sm.Errors))
		for _, e := range sm.Errors {
			row, col := e.Position()
			keys = append(keys, fmt.Sprintf("%s (line %d, col %d)", strings.Join(e.Key(), "."), row, col))
		}
		return &unknownKeysError{keys: keys}
	}
	return err
}

// projectFile is every key a project file may set: cue phrases, and the
// routing switches only in the careful direction (checkProject).
type projectFile struct {
	Routing struct {
		Mode            *model.Mode `toml:"mode"`
		AskOnUpgrade    *bool       `toml:"ask_on_upgrade"`
		ContextHold     *bool       `toml:"context_hold"`
		RetryEscalation *bool       `toml:"retry_escalation"`
	} `toml:"routing"`
	LexiconExtra map[string]model.LexiconExtra `toml:"lexicon_extra"`
}

// projectKeys are the dotted keys of projectFile; a prefix ending in "."
// admits every key below it.
var projectKeys = []string{"routing.mode", "routing.ask_on_upgrade", "routing.context_hold", "routing.retry_escalation", "lexicon_extra."}

// leafKeys lists the dotted paths of every value in a decoded TOML table
// (tables are walked; arrays and scalars are leaves).
func leafKeys(prefix string, table map[string]any) []string {
	var out []string
	for k, v := range table {
		if sub, ok := v.(map[string]any); ok {
			out = append(out, leafKeys(prefix+k+".", sub)...)
			continue
		}
		out = append(out, prefix+k)
	}
	return out
}

// checkProject rejects a project file that sets a key outside projectFile or
// relaxes a safeguard.
func checkProject(raw []byte) error {
	var table map[string]any
	if err := decode(raw, &table, false); err != nil {
		return err
	}
	var denied []string
	for _, key := range leafKeys("", table) {
		if !slices.ContainsFunc(projectKeys, func(allowed string) bool {
			return key == allowed || strings.HasSuffix(allowed, ".") && strings.HasPrefix(key, allowed)
		}) {
			denied = append(denied, key)
		}
	}
	if len(denied) > 0 {
		slices.Sort(denied)
		return fmt.Errorf("key(s) a project file may not set (misspelt, or reserved for ~/.claude/%s): %s", FileName, strings.Join(denied, ", "))
	}
	var pf projectFile
	if err := decode(raw, &pf, false); err != nil {
		return err
	}
	r := pf.Routing
	if r.Mode != nil && *r.Mode != model.ModeShadow && *r.Mode != model.ModeOff {
		return fmt.Errorf("routing.mode = %q: a project file may only choose %q or %q", *r.Mode, model.ModeShadow, model.ModeOff)
	}
	for _, sw := range []struct {
		key string
		v   *bool
	}{{"ask_on_upgrade", r.AskOnUpgrade}, {"context_hold", r.ContextHold}, {"retry_escalation", r.RetryEscalation}} {
		if sw.v != nil && !*sw.v {
			return fmt.Errorf("routing.%s = false: a project file may only turn this safeguard on", sw.key)
		}
	}
	return nil
}

// Load decodes the defaults, the user file and the project file of cwd. It
// returns the applied files and a problem ("path: reason") per rejected file.
// Every candidate is decoded afresh on top of the defaults and the layers
// accepted so far (never on a shallow copy, whose maps would be shared), so a
// rejected file cannot leak partial values.
func Load(cwd string) (model.Settings, []string, []string) { return load(modelrouter.Data, cwd, true) }

// LoadUser decodes the defaults and the user file only: the settings a whole
// process is wired with (data directory, ledger, artifact, HTTP address,
// limits), which no project file may influence.
func LoadUser() (model.Settings, []string, []string) { return load(modelrouter.Data, "", false) }

// load is Load over an arbitrary data file system (tests feed broken defaults).
func load(data fs.ReadFileFS, cwd string, withProject bool) (model.Settings, []string, []string) {
	defaults, err := data.ReadFile("data/defaults.toml")
	if err != nil {
		panic(fmt.Sprintf("embedded defaults.toml is missing: %v", err)) // a build defect, caught by tests
	}
	layers := [][]byte{defaults}
	build := func(extra ...[]byte) (model.Settings, error) {
		var st model.Settings
		for _, raw := range append(slices.Clone(layers), extra...) {
			if err := decode(raw, &st, true); err != nil {
				return model.Settings{}, err
			}
		}
		if err := st.Validate(); err != nil {
			return model.Settings{}, err
		}
		if err := st.ValidateLexiconExtra(); err != nil {
			return model.Settings{}, err
		}
		return st, nil
	}
	if _, err := build(); err != nil {
		panic(fmt.Sprintf("embedded defaults.toml is invalid: %v", err))
	}
	type candidate struct {
		path  string
		check func([]byte) error
	}
	candidates := []candidate{{path: UserPath()}}
	if withProject {
		candidates = append(candidates, candidate{path: ProjectPath(cwd), check: checkProject})
	}
	var used, problems []string
	for _, c := range candidates {
		raw, err := os.ReadFile(c.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err == nil && c.check != nil {
			err = c.check(raw)
		}
		if err == nil {
			_, err = build(raw)
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", c.path, err))
			continue
		}
		layers = append(layers, raw)
		used = append(used, c.path)
	}
	st, _ := build()
	return st, used, problems
}

// Tiers loads the embedded tier table (the single source of truth).
func Tiers() model.TierTable {
	var t model.TierTable
	mustDecode(modelrouter.Data, "data/tiers.toml", &t)
	return t
}

// Prices loads the embedded price table.
func Prices() service.PriceTable {
	var p service.PriceTable
	mustDecode(modelrouter.Data, "data/pricing.toml", &p)
	return p
}

// Lexicon loads the embedded lexicon, appends lexicon_extra, and compiles it
// with the classifier settings.
func Lexicon(st model.Settings) *service.Lexicon { return lexicon(modelrouter.Data, st) }

// lexicon is Lexicon over an arbitrary data file system.
func lexicon(data fs.ReadFileFS, st model.Settings) *service.Lexicon {
	lex := &service.Lexicon{}
	mustDecode(data, "data/lexicon.toml", lex)
	if lex.Classes == nil {
		lex.Classes = map[model.Class]service.PhraseSet{}
	}
	for name, extra := range st.LexiconExtra {
		switch name {
		case "danger":
			lex.Danger = append(lex.Danger, extra.Items...)
		case "continuation":
			lex.Continuation = append(lex.Continuation, extra.Items...)
		case "context":
			lex.Context = append(lex.Context, extra.Items...)
		case "correction":
			lex.Correction = append(lex.Correction, extra.Items...)
		default:
			cur := lex.Classes[model.Class(name)]
			cur.Strong, cur.Weak = append(cur.Strong, extra.Strong...), append(cur.Weak, extra.Weak...)
			lex.Classes[model.Class(name)] = cur
		}
	}
	return lex.Compile(st.Classifier)
}

func mustDecode(data fs.ReadFileFS, path string, v any) {
	raw, err := data.ReadFile(path)
	if err == nil {
		err = decode(raw, v, true)
	}
	if err != nil {
		panic(fmt.Sprintf("embedded %s is invalid: %v", path, err)) // a build defect, caught by tests
	}
}

// DataDir is runtime.data_dir, else PluginDataDir.
func DataDir(st model.Settings) string {
	return fsutil.ExpandHome(cmp.Or(st.Runtime.DataDir, PluginDataDir()))
}

// PluginDataDir is ${CLAUDE_PLUGIN_DATA}, else the default. The launcher
// (bin/pmr) knows no TOML, so the files it writes live here even when
// runtime.data_dir moves the ledger elsewhere.
func PluginDataDir() string {
	return fsutil.ExpandHome(cmp.Or(os.Getenv("CLAUDE_PLUGIN_DATA"), "~/.claude/plugins/data/promari-model-router"))
}

// LauncherErrorFile is the file bin/pmr writes in PluginDataDir when it
// cannot start the binary (and deletes after a successful start). The name
// is a contract with the launcher script, not a setting.
const LauncherErrorFile = "launcher_error"
