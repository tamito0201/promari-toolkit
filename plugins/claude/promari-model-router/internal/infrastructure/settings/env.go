package settings

import (
	"encoding/json"
	"os"
	"path/filepath"

	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/infrastructure/fsutil"
)

// EnvSettings implements repository.SettingsReader from the process
// environment and Claude Code's settings files.
type EnvSettings struct{}

var _ repository.SettingsReader = EnvSettings{}

// ProcessEnv implements repository.EnvReader over the process environment.
type ProcessEnv struct{}

var _ repository.EnvReader = ProcessEnv{}

// Getenv returns the variable, or "" when it is unset.
func (ProcessEnv) Getenv(key string) string { return os.Getenv(key) }

// EnvModel returns ANTHROPIC_MODEL.
func (EnvSettings) EnvModel() (string, bool) {
	v := os.Getenv("ANTHROPIC_MODEL")
	return v, v != ""
}

// SettingsModel returns the first `model` in local, project, then user settings.
func (EnvSettings) SettingsModel(cwd string) (string, bool) {
	project := ProjectPath(cwd)
	dir := filepath.Dir(project)
	for _, path := range []string{
		filepath.Join(dir, "settings.local.json"),
		filepath.Join(dir, "settings.json"),
		fsutil.ExpandHome("~/.claude/settings.json"),
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(raw, &s) == nil && s.Model != "" {
			return s.Model, true
		}
	}
	return "", false
}
