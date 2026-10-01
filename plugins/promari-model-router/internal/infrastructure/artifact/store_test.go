package artifact_test

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/artifact"
)

// ready is the smallest artifact whose Ready() is true.
func ready(origin string) model.Artifact {
	spec := model.FeatureSpec{HashBuckets: 4}
	return model.Artifact{
		Version: "t", Samples: 7, Origin: origin, Features: spec,
		Classes: []model.Class{model.ClassLookup}, Weights: [][]float64{make([]float64, spec.Dim())}, Bias: []float64{0},
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestStoreLoad(t *testing.T) {
	embedded, err := artifact.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	type got struct {
		Origin  string
		Samples int
		Broken  bool   // the error wraps model.ErrArtifactBroken
		Reason  string // a substring of the error
	}
	tests := []struct {
		name  string
		local []byte // nil: no local file
		dir   bool   // the local path is a directory (unreadable as a file)
		want  got
	}{
		{name: "no local file falls back to the embedded one", want: got{Origin: model.OriginEmbedded, Samples: embedded.Samples}},
		{
			name:  "local file without origin is local",
			local: mustJSON(t, ready("")),
			want:  got{Origin: model.OriginLocal, Samples: 7},
		},
		{
			name:  "local file keeps the origin recorded at training time",
			local: mustJSON(t, ready(model.OriginEmbedded)),
			want:  got{Origin: model.OriginEmbedded, Samples: 7},
		},
		// A broken local artifact used to be skipped without a word, so a
		// routing that silently ran on rules only looked healthy.
		{
			name:  "broken JSON falls back to the embedded one and says why",
			local: []byte("{not json"),
			want:  got{Origin: model.OriginEmbedded, Samples: embedded.Samples, Broken: true, Reason: "invalid character"},
		},
		{
			name:  "valid but not ready falls back to the embedded one and says why",
			local: mustJSON(t, model.Artifact{Samples: 3}),
			want:  got{Origin: model.OriginEmbedded, Samples: embedded.Samples, Broken: true, Reason: "artifact.json: no classes"},
		},
		{
			name: "an unreadable local file falls back and says why", dir: true,
			want: got{Origin: model.OriginEmbedded, Samples: embedded.Samples, Broken: true, Reason: "is a directory"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "artifact.json")
			if tt.local != nil {
				if err := os.WriteFile(path, tt.local, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tt.dir {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			a, err := artifact.Store{LocalPath: path}.Load()
			reason := ""
			if err != nil && strings.Contains(err.Error(), tt.want.Reason) {
				reason = tt.want.Reason
			}
			if diff := cmp.Diff(tt.want, got{a.Origin, a.Samples, errors.Is(err, model.ErrArtifactBroken), reason}); diff != "" {
				t.Errorf("Load() mismatch (-want +got):\n%s\nerror: %v", diff, err)
			}
		})
	}
}

func TestEmbedded(t *testing.T) {
	tests := []struct {
		name       string
		fsys       fstest.MapFS // nil: the real embedded data
		wantErr    string
		wantOrigin string
	}{
		{name: "the shipped artifact is ready and marked embedded", wantOrigin: model.OriginEmbedded},
		{name: "missing file asks for pmr train", fsys: fstest.MapFS{}, wantErr: "no embedded artifact: run `pmr train`"},
		{
			name: "broken file is an error", fsys: fstest.MapFS{"data/artifact.json": {Data: []byte("[")}},
			wantErr: "unexpected end of JSON input",
		},
		{
			name: "origin is overwritten to embedded", fsys: fstest.MapFS{"data/artifact.json": {Data: []byte(`{"origin":"local"}`)}},
			wantOrigin: model.OriginEmbedded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				a   model.Artifact
				err error
			)
			if tt.fsys == nil {
				a, err = artifact.Embedded()
				if err == nil && !a.Ready() {
					t.Error("embedded artifact is not ready")
				}
			} else {
				a, err = artifact.EmbeddedFrom(tt.fsys)
			}
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if diff := cmp.Diff(tt.wantErr, gotErr); diff != "" {
				t.Errorf("error mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantOrigin, a.Origin); diff != "" {
				t.Errorf("origin mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStoreSave(t *testing.T) {
	nan := ready("")
	nan.Weights = [][]float64{{math.NaN()}}
	tests := []struct {
		name    string
		path    func(t *testing.T, dir string) string // prepares the directory, returns LocalPath
		in      model.Artifact
		wantErr bool
	}{
		{
			name: "writes 0600 into a new directory and round-trips",
			path: func(_ *testing.T, dir string) string { return filepath.Join(dir, "sub", "artifact.json") },
			in:   ready(""),
		},
		{
			name: "unmarshalable artifact (NaN weight)", in: nan, wantErr: true,
			path: func(_ *testing.T, dir string) string { return filepath.Join(dir, "artifact.json") },
		},
		{
			name: "parent is a file", in: ready(""), wantErr: true,
			path: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file")
				if err := os.WriteFile(file, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(file, "artifact.json")
			},
		},
		{
			name: "the directory is not writable", in: ready(""), wantErr: true,
			path: func(t *testing.T, dir string) string {
				t.Helper()
				ro := filepath.Join(dir, "ro")
				if err := os.Mkdir(ro, 0o500); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(ro, "artifact.json")
			},
		},
		{
			name: "rename over a non-empty directory fails", in: ready(""), wantErr: true,
			path: func(t *testing.T, dir string) string {
				t.Helper()
				path := filepath.Join(dir, "artifact.json")
				if err := os.MkdirAll(filepath.Join(path, "occupied"), 0o700); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := artifact.Store{LocalPath: tt.path(t, t.TempDir())}
			got, err := store.Save(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Save() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if diff := cmp.Diff(store.LocalPath, got); diff != "" {
				t.Errorf("path mismatch (-want +got):\n%s", diff)
			}
			info, err := os.Stat(got)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(os.FileMode(0o600), info.Mode().Perm()); diff != "" {
				t.Errorf("mode mismatch (-want +got):\n%s", diff)
			}
			back, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff([]any{model.OriginLocal, 7}, []any{back.Origin, back.Samples}); diff != "" {
				t.Errorf("round trip mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
