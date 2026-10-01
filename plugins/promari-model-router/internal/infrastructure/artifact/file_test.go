package artifact_test

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/artifact"
)

func TestFileSave(t *testing.T) {
	tests := []struct {
		name    string
		path    func(t *testing.T) string
		art     model.Artifact
		wantErr bool
	}{
		{name: "written", path: func(t *testing.T) string {
			t.Helper()
			return filepath.Join(t.TempDir(), "a.json")
		}, art: model.Artifact{Version: "v1"}},
		{
			name: "NaN cannot be encoded", path: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "a.json")
			},
			art: model.Artifact{Metrics: map[string]float64{"x": math.NaN()}}, wantErr: true,
		},
		{
			name: "directory does not exist", path: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "no", "a.json")
			},
			art: model.Artifact{Version: "v1"}, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.path(t)
			got, err := artifact.File{Path: p}.Save(tt.art)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if diff := cmp.Diff(p, got); diff != "" {
				t.Errorf("path (-want +got):\n%s", diff)
			}
			raw, err := os.ReadFile(got)
			if err != nil || !strings.HasSuffix(string(raw), "\n") || !strings.Contains(string(raw), `"version": "v1"`) {
				t.Errorf("file %s = %q, %v", got, raw, err)
			}
			info, err := os.Stat(got)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Errorf("mode = %v, %v; want 0600", info, err)
			}
		})
	}
}
