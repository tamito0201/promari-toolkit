package artifact

import (
	"io/fs"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
)

// Test-only hooks into unexported helpers (compiled only with the tests).

// EmbeddedFrom exposes embedded over an arbitrary file system.
func EmbeddedFrom(fsys fs.ReadFileFS) (model.Artifact, error) { return embedded(fsys) }
