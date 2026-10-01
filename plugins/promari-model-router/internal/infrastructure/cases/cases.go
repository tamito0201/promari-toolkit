// Package cases reads labelled prompts (JSONL: one {lang, expect, text,
// danger} object per line) for `pmr eval` and `pmr train`. The empty path is
// the evaluation set embedded in the binary.
package cases

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"

	modelrouter "github.com/tamito0201/promari-toolkit/plugins/promari-model-router"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/learn"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
)

// EmbeddedPath is the evaluation set inside the embedded data.
const EmbeddedPath = "data/eval_set.jsonl"

// Line is one line of a labelled JSONL file.
type Line struct {
	Lang   string `json:"lang"`
	Expect string `json:"expect"`
	Text   string `json:"text"`
	Danger bool   `json:"danger"`
}

// Source implements repository.CaseSource over the embedded data and the
// local file system.
type Source struct {
	// Embedded holds EmbeddedPath (the binary's data in production).
	Embedded fs.FS
}

var _ repository.CaseSource = Source{}

// New reads the embedded set from the binary.
func New() Source { return Source{Embedded: modelrouter.Data} }

// Cases parses the file at path; "" is the embedded evaluation set. A line
// of any length is accepted (the file is already in memory); a malformed line
// is reported as "<path>:<line>: ...".
func (s Source) Cases(path string) ([]learn.Case, error) {
	var raw []byte
	var err error
	name := path
	if path == "" {
		name = EmbeddedPath
		raw, err = fs.ReadFile(s.Embedded, EmbeddedPath)
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	var out []learn.Case
	n := 0
	for line := range bytes.Lines(raw) {
		n++
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var c Line
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, fmt.Errorf("%s:%d: parse labelled line: %w", name, n, err)
		}
		out = append(out, learn.Case{Text: c.Text, Expect: model.ParseClass(c.Expect), Danger: c.Danger, Lang: c.Lang})
	}
	return out, nil
}
