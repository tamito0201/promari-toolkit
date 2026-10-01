// Package artifact loads and saves the learned routing artifact. A local
// artifact written by `pmr train` (${CLAUDE_PLUGIN_DATA}/artifact.json) wins
// over the one embedded in the binary; the local one may be trained on the
// user's own prompts, so it never leaves the machine.
package artifact

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	modelrouter "github.com/tamito0201/promari-toolkit/plugins/promari-model-router"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/fsutil"
)

// Store implements repository.ArtifactStore.
type Store struct {
	LocalPath string
}

var _ repository.ArtifactStore = Store{}

// Load returns the local artifact if present and valid, else the embedded
// one. A local file that exists but cannot be used is not skipped silently:
// the embedded artifact comes back with an error wrapping
// model.ErrArtifactBroken and the reason, so the hook can mark its decision
// degraded and `pmr doctor` can say the local artifact is broken.
func (s Store) Load() (model.Artifact, error) {
	raw, err := os.ReadFile(s.LocalPath)
	if errors.Is(err, fs.ErrNotExist) {
		return Embedded()
	}
	var a model.Artifact
	if err == nil {
		err = json.Unmarshal(raw, &a)
	}
	if err == nil {
		err = a.Validate()
	}
	if err != nil {
		emb, embErr := Embedded()
		return emb, errors.Join(fmt.Errorf("%w: %s: %w", model.ErrArtifactBroken, s.LocalPath, err), embErr)
	}
	// Keep the origin recorded at training time: an artifact trained on the
	// embedded synthetic set stays "embedded" even when saved here. Files
	// written before origins were recorded came from --file data.
	a.Origin = cmp.Or(a.Origin, model.OriginLocal)
	return a, nil
}

// Embedded returns the artifact shipped in the binary.
func Embedded() (model.Artifact, error) { return embedded(modelrouter.Data) }

// embedded decodes data/artifact.json from fsys (split out so tests can feed
// a missing or broken file; the real one is embedded and always present).
func embedded(fsys fs.ReadFileFS) (model.Artifact, error) {
	raw, err := fsys.ReadFile("data/artifact.json")
	if err != nil {
		return model.Artifact{}, errors.New("no embedded artifact: run `pmr train`")
	}
	var a model.Artifact
	if err := json.Unmarshal(raw, &a); err != nil {
		return model.Artifact{}, err
	}
	a.Origin = model.OriginEmbedded
	return a, nil
}

// Save writes the artifact atomically (a temporary file flushed and renamed)
// with mode 0600.
func (s Store) Save(a model.Artifact) (string, error) {
	raw, err := json.Marshal(a)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(s.LocalPath), 0o700); err != nil {
		return "", err
	}
	return s.LocalPath, fsutil.WriteFileAtomic(s.LocalPath, raw)
}
