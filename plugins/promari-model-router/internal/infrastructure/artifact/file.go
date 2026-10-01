package artifact

import (
	"encoding/json"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/fsutil"
)

// File writes an artifact to an explicit path (`pmr train --output`). It is
// write-only: the file is an export (indented, for review and for the
// embedded data/artifact.json), not a store the router loads from, so it
// implements repository.ArtifactWriter and nothing else.
type File struct {
	Path string
}

var _ repository.ArtifactWriter = File{}

// Save writes the artifact as indented JSON with mode 0600, atomically (a
// temporary file flushed and renamed), so an interrupted export never leaves
// a truncated artifact.json behind.
func (f File) Save(a model.Artifact) (string, error) {
	raw, err := json.MarshalIndent(a, "", " ")
	if err != nil {
		return "", err
	}
	return f.Path, fsutil.WriteFileAtomic(f.Path, append(raw, '\n'))
}
