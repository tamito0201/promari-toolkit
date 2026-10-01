// Package agents reads the fixed-tier agent definitions embedded in the
// binary (agents/*.md), which `pmr lint` and `pmr doctor` check against
// data/tiers.toml. It owns the file format: the use cases see only the model
// and effort the frontmatter declares.
package agents

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	modelrouter "github.com/tamito0201/promari-toolkit/plugins/promari-model-router"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
)

var frontmatterRE = regexp.MustCompile(`(?s)^---\n(.*?)\n---`)

// Source implements repository.AgentSource over a file system laid out like
// the plugin (agents/<name>.md).
type Source struct {
	FS fs.FS
}

var _ repository.AgentSource = Source{}

// New reads the definitions embedded in the binary.
func New() Source { return Source{FS: modelrouter.Agents} }

// AgentSpec parses the `model:` and `effort:` lines of the frontmatter of
// agents/<name>.md (a flat "key: value" block; other keys are ignored).
func (s Source) AgentSpec(name string) (model.AgentSpec, error) {
	raw, err := fs.ReadFile(s.FS, "agents/"+name+".md")
	if err != nil {
		return model.AgentSpec{}, err
	}
	m := frontmatterRE.FindSubmatch(raw)
	if m == nil {
		return model.AgentSpec{}, fmt.Errorf("agents/%s.md: %w", name, repository.ErrNoFrontmatter)
	}
	fm := map[string]string{}
	for line := range strings.SplitSeq(string(m[1]), "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok {
			fm[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return model.AgentSpec{Model: model.Tier(fm["model"]), Effort: fm["effort"]}, nil
}
