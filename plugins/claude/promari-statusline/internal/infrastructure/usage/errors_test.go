package usage_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/infrastructure/platform/platformtest"
	"promari-statusline/internal/infrastructure/usage"
)

func TestBoardRefusesWhatJSONCannotHold(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC)
	sys := platformtest.New(at)
	board := usage.Board{Sys: sys}
	nan := math.NaN()
	if err := board.PostClaude(model.RateLimits{FiveHour: model.Some(model.RateWindow{UsedPct: nan})}, at); err == nil {
		t.Error("PostClaude() of NaN is an error")
	}
	if err := board.PostCodex(model.CodexLimits{Primary: model.Some(model.CodexWindow{UsedPct: nan})}, at); err == nil {
		t.Error("PostCodex() of NaN is an error")
	}
	if len(sys.Files) != 0 {
		t.Errorf("nothing is written: %v", sys.Files)
	}
}

func TestCodexLogThatCannotBeRead(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC))
	log := sys.HomeDir() + "/.codex/sessions/2026/10/03/rollout.jsonl"
	sys.Files[log] = []byte("{}\n")
	sys.ReadErr = map[string]error{log: errors.New("denied")}
	if _, err := (usage.Codex{Sys: sys}).Codex(context.Background()); err == nil {
		t.Error("a log that cannot be read is an error")
	}
}
