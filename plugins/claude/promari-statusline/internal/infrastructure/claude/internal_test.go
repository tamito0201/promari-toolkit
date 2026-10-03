package claude

import (
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/infrastructure/platform/platformtest"
	"promari-statusline/pkg/jsonx"
)

func TestCallKindOfASearch(t *testing.T) {
	t.Parallel()
	for _, name := range searchTools {
		if callKind(name) != model.CallSearch {
			t.Errorf("%s is a search", name)
		}
	}
}

func TestReadResultsSkipsOtherBlocks(t *testing.T) {
	t.Parallel()
	var tr model.Transcript
	tr.Quality.Await(model.PendingCall{ID: "t1", Check: model.CheckTest})
	entry, _ := jsonx.Parse([]byte(`{"message":{"content":[{"type":"text","text":"x"},{"type":"tool_result","tool_use_id":"t1","content":"ok  pkg 0.1s"}]}}`))
	readResults(&tr, entry)
	if len(tr.Quality.Pending) != 0 {
		t.Errorf("the tool result after a text block is read: pending %v", tr.Quality.Pending)
	}
}

func TestEditRefusesMembersThatCannotBeEncoded(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC))
	s := Settings{Sys: sys}
	_, err := s.edit(func([]member) []member { return []member{{name: "\xff", value: []byte(`1`)}} })
	if err == nil {
		t.Error("a member that cannot be encoded fails the edit")
	}
	if _, wrote := sys.Files[s.Path()]; wrote {
		t.Error("nothing is written")
	}
}
