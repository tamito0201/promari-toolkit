package modelrouter_test

import (
	"embed"
	"io/fs"
	"testing"

	"github.com/google/go-cmp/cmp"

	modelrouter "github.com/tamito0201/promari-toolkit/plugins/promari-model-router"
)

func TestEmbeddedFiles(t *testing.T) {
	tests := []struct {
		name string
		fsys embed.FS
		glob string
		want []string
	}{
		{
			name: "toml configuration and tables",
			fsys: modelrouter.Data, glob: "data/*.toml",
			want: []string{"data/defaults.toml", "data/lexicon.toml", "data/pricing.toml", "data/tiers.toml"},
		},
		{name: "evaluation set", fsys: modelrouter.Data, glob: "data/*.jsonl", want: []string{"data/eval_set.jsonl"}},
		{name: "learned artifact", fsys: modelrouter.Data, glob: "data/*.json", want: []string{"data/artifact.json"}},
		{
			name: "fixed-tier agent definitions",
			fsys: modelrouter.Agents, glob: "agents/*.md",
			want: []string{"agents/architect.md", "agents/engineer.md", "agents/scout.md", "agents/worker.md"},
		},
		{name: "agents are not in Data", fsys: modelrouter.Data, glob: "agents/*.md", want: nil},
		{name: "data is not in Agents", fsys: modelrouter.Agents, glob: "data/*", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fs.Glob(tt.fsys, tt.glob)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			for _, name := range got {
				b, err := fs.ReadFile(tt.fsys, name)
				if err != nil {
					t.Fatalf("read %s: %v", name, err)
				}
				if len(b) == 0 {
					t.Errorf("%s is empty", name)
				}
			}
		})
	}
}
