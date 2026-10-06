package claude_test

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/infrastructure/claude"
	"promari-statusline/internal/infrastructure/platform/platformtest"
)

// update rewrites the golden files from what the reader returns now:
// go test ./internal/infrastructure/claude -run TestTranscriptGolden -update
var update = flag.Bool("update", false, "rewrite the golden files of the transcript reader")

// TestTranscriptGolden pins what a reading of each recorded transcript holds,
// member by member. It is the characterization the reader was refactored
// against: a change of what is counted shows here as a diff of the golden file.
func TestTranscriptGolden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// cuts are the byte offsets at which the file is read before it is read
		// whole: a read that stops part way and continues must count the same.
		cuts []int
	}{
		{name: "recorded"},
		{name: "quality"},
		{name: "claims"},
		{name: "research"},
		{name: "observations"},
		{name: "observations", cuts: []int{200, 700, 1100}},
	}
	for _, tt := range tests {
		golden := filepath.Join("testdata", tt.name+".golden.json")
		t.Run(filepath.Base(golden)+cutsName(tt.cuts), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(filepath.Join("testdata", tt.name+".jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			got := readGolden(t, data, tt.cuts)
			if *update && len(tt.cuts) == 0 {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("the reading of %s.jsonl differs from %s:\n%s", tt.name, golden, got)
			}
		})
	}
}

// readGolden reads data as a transcript, first up to each cut and then whole,
// and returns the reading as indented, deterministic JSON.
func readGolden(t *testing.T, data []byte, cuts []int) []byte {
	t.Helper()
	sys := platformtest.New(t0)
	reader := claude.Transcript{Sys: sys}
	var read model.Transcript
	for _, cut := range append(cuts, len(data)) {
		sys.Files["/t.jsonl"] = data[:cut]
		read, _ = reader.Transcript(t.Context(), "/t.jsonl", read)
	}
	out, err := json.Marshal(read, json.Deterministic(true), jsontext.Multiline(true), jsontext.WithIndent("  "))
	if err != nil {
		t.Fatal(err)
	}
	return append(out, '\n')
}

func cutsName(cuts []int) string {
	if len(cuts) == 0 {
		return ""
	}
	return " read in parts"
}
