package cases_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/learn"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/infrastructure/cases"
)

// writeFile writes content to a fresh file and returns its path.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cases.jsonl")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSourceCases(t *testing.T) {
	tests := []struct {
		name    string
		path    func(t *testing.T) string
		want    []learn.Case
		wantMin int // > 0: only the count is checked (the embedded set)
		src     cases.Source
		wantErr func(error) bool
	}{
		{name: "embedded evaluation set", path: func(t *testing.T) string {
			t.Helper()
			return ""
		}, wantMin: 10, src: cases.New()},
		{
			name: "embedded set missing from the data",
			path: func(t *testing.T) string {
				t.Helper()
				return ""
			},
			src:     cases.Source{Embedded: fstest.MapFS{}},
			wantErr: func(err error) bool { return errors.Is(err, fs.ErrNotExist) },
		},
		{
			name: "file with blank lines",
			path: func(t *testing.T) string {
				t.Helper()
				return writeFile(t, `{"lang":"ja","expect":"lookup","text":"探して"}`+"\n\n   \n"+`{"lang":"en","expect":"","text":"hi","danger":true}`+"\n")
			},
			want: []learn.Case{
				{Text: "探して", Expect: model.ClassLookup, Lang: "ja"},
				{Text: "hi", Expect: model.ClassNone, Lang: "en", Danger: true},
			},
		},
		{
			name: "missing file",
			path: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "absent.jsonl")
			},
			wantErr: func(err error) bool { return errors.Is(err, fs.ErrNotExist) },
		},
		{
			// The error used to say only "parse eval line": which file, which line?
			name: "a malformed line names the file and the line",
			path: func(t *testing.T) string {
				t.Helper()
				return writeFile(t, `{"lang":"ja","expect":"lookup","text":"探して"}`+"\n\n{not json}\n")
			},
			wantErr: func(err error) bool {
				return err != nil && strings.HasSuffix(strings.SplitN(err.Error(), ": parse labelled line", 2)[0], "cases.jsonl:3")
			},
		},
		{
			// A typo used to be read as "abstain", and learned as such.
			name: "a class name that is not a class names the file and the line",
			path: func(t *testing.T) string {
				t.Helper()
				return writeFile(t, `{"lang":"ja","expect":"lookup","text":"探して"}`+"\n"+`{"lang":"ja","expect":"standrad","text":"実装して"}`+"\n")
			},
			wantErr: func(err error) bool {
				return err != nil && strings.Contains(err.Error(), `cases.jsonl:2: unknown class "standrad"`)
			},
		},
		{
			name: "abstain is the word for no class",
			path: func(t *testing.T) string {
				t.Helper()
				return writeFile(t, `{"lang":"en","expect":"abstain","text":"hi"}`+"\n")
			},
			wantErr: func(err error) bool { return err == nil },
		},
		{
			name: "a malformed line of the embedded set names it",
			path: func(t *testing.T) string {
				t.Helper()
				return ""
			},
			src: cases.Source{Embedded: fstest.MapFS{cases.EmbeddedPath: {Data: []byte("[\n")}}},
			wantErr: func(err error) bool {
				return err != nil && strings.HasPrefix(err.Error(), cases.EmbeddedPath+":1: parse labelled line")
			},
		},
		{
			// A line longer than the old 16 MiB scanner limit failed the whole file.
			name: "a line of any length",
			path: func(t *testing.T) string {
				t.Helper()
				return writeFile(t, `{"text":"`+strings.Repeat("x", 64)+`"}`)
			},
			want: []learn.Case{{Text: strings.Repeat("x", 64)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := tt.src
			if src.Embedded == nil {
				src = cases.New()
			}
			got, err := src.Cases(tt.path(t))
			if tt.wantErr != nil {
				if !tt.wantErr(err) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantMin > 0 {
				if len(got) < tt.wantMin {
					t.Errorf("cases = %d, want >= %d", len(got), tt.wantMin)
				}
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("cases (-want +got):\n%s", diff)
			}
		})
	}
}
