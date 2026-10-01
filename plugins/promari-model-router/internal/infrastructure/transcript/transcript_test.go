package transcript_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/transcript"
)

func assistant(model string) string {
	return `{"type":"assistant","message":{"model":"` + model + `"}}`
}

func TestLatestModel(t *testing.T) {
	const sidechain = `{"type":"assistant","isSidechain":true,"message":{"model":"claude-haiku-4-5"}}`
	type result struct {
		Model string
		OK    bool
	}
	tests := []struct {
		name string
		tail int64
		// body is the JSONL content; the file is not created when nil.
		body *string
		// path overrides the transcript path ("<file>" = the temp file, "<dir>" = its directory).
		path string
		want result
	}{
		{name: "empty path", path: "", want: result{}},
		{name: "missing file", path: "<file>", want: result{}},
		{name: "a directory cannot be read", path: "<dir>", want: result{}},
		{
			name: "the latest main-thread assistant wins", path: "<file>",
			body: new(strings.Join([]string{assistant("claude-sonnet-4-6"), `{"type":"user"}`, assistant("claude-opus-5-5"), ""}, "\n")),
			want: result{"claude-opus-5-5", true},
		},
		{
			name: "subagent (sidechain) lines are skipped", path: "<file>",
			body: new(assistant("claude-opus-5-5") + "\n" + sidechain),
			want: result{"claude-opus-5-5", true},
		},
		{
			name: "broken and non-assistant lines are skipped", path: "<file>",
			body: new(assistant("claude-opus-5-5") + "\n" + `{"type":"user","text":"assistant","model":"x"}` + "\n" + `{"type":"assistant","model": broken`),
			want: result{"claude-opus-5-5", true},
		},
		{
			name: "synthetic and empty models are skipped", path: "<file>",
			body: new(assistant("claude-opus-5-5") + "\n" + assistant("<synthetic>") + "\n" + assistant("")),
			want: result{"claude-opus-5-5", true},
		},
		{
			name: "no assistant message yet (first turn)", path: "<file>",
			body: new(`{"type":"user","message":{"content":"hi"}}` + "\n" + sidechain), want: result{},
		},
		{
			name: "only the tail is read: a model before it is not seen", path: "<file>", tail: 40,
			body: new(assistant("claude-opus-5-5") + "\n" + strings.Repeat(`{"type":"user"}`+"\n", 5)),
			want: result{},
		},
		{
			name: "only the tail is read: a model inside it is found", path: "<file>", tail: 60,
			body: new(strings.Repeat(`{"type":"user"}`+"\n", 5) + assistant("claude-sonnet-4-6")),
			want: result{"claude-sonnet-4-6", true},
		},
		{
			name: "a tail larger than the file reads everything", path: "<file>", tail: 1 << 20,
			body: new(assistant("claude-sonnet-4-6")), want: result{"claude-sonnet-4-6", true},
		},
		{
			name: "a leading ~/ is expanded to HOME", path: "~/t.jsonl",
			body: new(assistant("claude-opus-5-5")), want: result{"claude-opus-5-5", true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			file := filepath.Join(dir, "t.jsonl")
			if tt.body != nil {
				if err := os.WriteFile(file, []byte(*tt.body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			path := strings.NewReplacer("<file>", file, "<dir>", dir).Replace(tt.path)
			m, ok := transcript.Reader{TailBytes: tt.tail}.LatestModel(path)
			if diff := cmp.Diff(tt.want, result{m, ok}); diff != "" {
				t.Errorf("LatestModel() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
