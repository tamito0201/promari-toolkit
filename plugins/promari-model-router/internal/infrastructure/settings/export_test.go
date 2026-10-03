package settings

import (
	"io/fs"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
)

// Test-only hooks into unexported helpers (compiled only with the tests).

// LoadFrom exposes load over an arbitrary data file system.
func LoadFrom(data fs.ReadFileFS, cwd string) (model.Settings, []string, []string) {
	return load(data, cwd, true)
}

// LexiconFrom exposes lexicon over an arbitrary data file system.
func LexiconFrom(data fs.ReadFileFS, st model.Settings) *service.Lexicon { return lexicon(data, st) }

// MustDecode exposes mustDecode.
func MustDecode(data fs.ReadFileFS, path string, v any) { mustDecode(data, path, v) }
