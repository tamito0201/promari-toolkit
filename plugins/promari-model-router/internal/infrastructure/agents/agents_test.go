package agents_test

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/agents"
)

func TestAgentSpec(t *testing.T) {
	tree := func(body string) agents.Source {
		return agents.Source{FS: fstest.MapFS{"agents/x.md": {Data: []byte(body)}}}
	}
	tests := []struct {
		name    string
		src     agents.Source
		agent   string
		want    model.AgentSpec
		wantErr error
	}{
		{name: "a shipped agent", src: agents.New(), agent: "scout", want: model.AgentSpec{Model: model.TierHaiku}},
		{
			name: "other keys and lines without a colon are ignored", src: tree("---\nname: x\nmodel: sonnet\neffort: high\nnot a pair\n---\nbody"),
			agent: "x", want: model.AgentSpec{Model: model.TierSonnet, Effort: "high"},
		},
		{name: "missing keys read as empty", src: tree("---\nname: x\n---\n"), agent: "x"},
		{name: "no frontmatter", src: tree("body"), agent: "x", wantErr: repository.ErrNoFrontmatter},
		{name: "an unknown agent is missing", src: agents.New(), agent: "nope", wantErr: fs.ErrNotExist},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.src.AgentSpec(tt.agent)
			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}
