// Package transcript reads the Claude Code session transcript (JSONL) to find
// the model that actually answered. That beats every configured value:
// --model and /model outrank ANTHROPIC_MODEL, and SessionStart does not always
// carry `model` (it is absent in -p runs). The first turn of a session has no
// assistant message yet, so callers must fall back when nothing is found.
package transcript

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"slices"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/fsutil"
)

// Reader implements repository.TranscriptReader.
type Reader struct {
	TailBytes int64
}

var _ repository.TranscriptReader = Reader{}

// LatestModel returns the model of the latest main-thread assistant message.
func (r Reader) LatestModel(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	f, err := os.Open(fsutil.ExpandHome(path))
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	tail := r.TailBytes
	if info, err := f.Stat(); err == nil && tail > 0 && info.Size() > tail {
		_, _ = f.Seek(info.Size()-tail, io.SeekStart)
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return "", false
	}
	lines := bytes.Split(buf, []byte("\n"))
	for _, line := range slices.Backward(lines) {
		if !bytes.Contains(line, []byte(`"assistant"`)) || !bytes.Contains(line, []byte(`"model"`)) {
			continue
		}
		var e struct {
			Type        string `json:"type"`
			IsSidechain bool   `json:"isSidechain"`
			Message     struct {
				Model string `json:"model"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &e) != nil || e.Type != "assistant" || e.IsSidechain {
			continue
		}
		if m := e.Message.Model; m != "" && m != "<synthetic>" {
			return m, true
		}
	}
	return "", false
}
