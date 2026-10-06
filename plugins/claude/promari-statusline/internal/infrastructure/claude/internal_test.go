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

func TestStructuredToolObservationsSurviveIncrementalEntries(t *testing.T) {
	t.Parallel()
	var tr model.Transcript
	for _, line := range []string{
		`{"type":"assistant","timestamp":"2026-10-06T01:00:00Z","message":{"id":"m","usage":{"input_tokens":10,"output_tokens":5},"content":[{"type":"tool_use","id":"a","name":"Read","input":{}},{"type":"tool_use","id":"b","name":"Bash","input":{}}]}}`,
		`{"type":"assistant","timestamp":"2026-10-06T01:00:00Z","message":{"id":"m","usage":{"input_tokens":10,"output_tokens":5},"content":[{"type":"tool_use","id":"a","name":"Read","input":{}}]}}`,
		`{"type":"assistant","isSidechain":true,"message":{"id":"side","content":[{"type":"tool_use","id":"side","name":"Read","input":{}}]}}`,
		`{"type":"user","isSidechain":true,"message":{"content":[{"type":"tool_result","tool_use_id":"side","is_error":true}]}}`,
		`{"type":"user","timestamp":"2026-10-06T01:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"b","is_error":true},{"type":"tool_result","tool_use_id":"a","content":"ok"}]}}`,
		`{"type":"user","timestamp":"2026-10-06T01:00:03Z","message":{"content":[{"type":"tool_result","tool_use_id":"a","content":"duplicate"}]}}`,
	} {
		readEntry(&tr, []byte(line))
	}
	o := tr.Observations.View()
	if tr.UsageRequests != 1 || tr.Tokens.Input != 10 || o.Calls != 2 || o.Completed != 2 || o.Failed != 1 || o.Unpaired != 0 || len(o.Seconds) != 2 || o.Seconds[0] != 2 {
		t.Fatalf("構造化イベントの対応: %+v / usage=%d", o, tr.UsageRequests)
	}
}
