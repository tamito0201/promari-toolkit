package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "success exits 0", args: []string{"--version"}, want: 0},
		{name: "an error exits 1", args: []string{"no-such-command"}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("HOME", filepath.Join(tmp, "home"))
			t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(tmp, "data"))
			t.Setenv("CLAUDE_PROJECT_DIR", filepath.Join(tmp, "project"))
			old := os.Args
			os.Args = append([]string{"pmr"}, tt.args...)
			t.Cleanup(func() { os.Args = old })
			if diff := cmp.Diff(tt.want, run()); diff != "" {
				t.Errorf("run() (-want +got):\n%s", diff)
			}
		})
	}
}
