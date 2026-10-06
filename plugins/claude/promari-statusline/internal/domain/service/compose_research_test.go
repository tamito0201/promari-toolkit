package service

import (
	"encoding/json/v2"
	"math"
	"strconv"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

// observationsJSON reads saved tool observations; the reading checks their invariants.
func observationsJSON(t *testing.T, saved string) model.ToolObservations {
	t.Helper()
	var o model.ToolObservations
	if err := json.Unmarshal([]byte(saved), &o); err != nil {
		t.Fatal(err)
	}
	return o
}

// timedCalls are n calls of one tool, each answered a second later.
func timedCalls(n int) model.ToolObservations {
	var o model.ToolObservations
	at := time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)
	for i := range n {
		id := "call" + strconv.Itoa(i)
		o.Called(model.ToolCall{ID: id, Name: "Read", At: at})
		o.Resulted(model.ToolResult{ID: id, At: at.Add(time.Second)})
	}
	return o
}

// manyNames are calls of one tool more than the names tracked.
func manyNames() model.ToolObservations {
	var o model.ToolObservations
	for i := range model.ToolKindsLimit + 1 {
		o.Called(model.ToolCall{ID: "call" + strconv.Itoa(i), Name: "tool" + strconv.Itoa(i)})
	}
	return o
}

func researchTranscript(t *testing.T) model.Transcript {
	t.Helper()
	turns := make([]float64, 100)
	for i := range turns {
		turns[i] = float64(i + 1)
	}
	return model.Transcript{
		Cursor: model.TranscriptCursor{Format: model.TranscriptFormat}, Turns: turns,
		UsageRequests: 10, Tokens: model.TokenTotals{Input: 100, CacheRead: 900, Output: 200},
		Trace: model.Trace{TurnTokens: turns},
		Hooks: 4, HookErrors: 1,
		Quality:      model.Quality{Tests: model.CheckRuns{Runs: 4, Passed: 2, Failed: 1}, Builds: model.CheckRuns{Runs: 3, Passed: 1, Failed: 1}, Edits: 5, EditFailures: 1},
		Observations: observationsJSON(t, `{"calls":102,"completed":100,"failed":20,"skipped":1,"pending":[{"id":"waiting"}],"names":{"Read":6,"Bash":6},"transitions":11,"switches":5,"seconds":`+secondsJSON(turns)+`}`),
	}
}

func secondsJSON(seconds []float64) string {
	b, err := json.Marshal(seconds)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestResearchMeasurementContract(t *testing.T) {
	t.Parallel()
	tr := researchTranscript(t)
	v := &View{Facts: model.Facts{Transcript: model.Some(tr)}}
	m := Measurements(v)
	want := map[string]float64{"turnSamples": 100, "turnMean": 50.5, "turnP95": 95, "turnP99": 99, "turnTail": 1.9, "toolErrorRate": 20, "toolPending": 1, "toolTimeMean": 50.5, "toolTimeP50": 50, "toolTimeP95": 95, "toolKinds": 2, "toolEffective": 2, "toolDominance": 50, "toolEntropy": 1, "inputPerRequest": 100, "outputPerRequest": 20, "sessionCacheShare": 90, "testDecided": 3, "testUnknown": 1, "testPassRate": 200.0 / 3, "buildUnknown": 1, "buildPassRate": 50, "hookErrorRate": 25, "editErrorRate": 20}
	for key, value := range want {
		if got, ok := m[key].Value.Get(); !ok || math.Abs(got-value) > 1e-9 {
			t.Errorf("%s=%+v want %v", key, m[key], value)
		}
	}
	count := 0
	for _, section := range researchSections {
		chips := section.chips(v)
		if len(chips) != len(section.fields) {
			t.Errorf("%s: チップ%d / 定義%d", section.category, len(chips), len(section.fields))
		}
		for _, field := range section.fields {
			count++
			if !m[field.key].Value.Present() {
				t.Errorf("集計が無い定義: %s", field.key)
			}
		}
	}
	if count != 49 {
		t.Fatalf("指標数=%d", count)
	}
}

func TestResearchMissingSparseZeroAndOldCache(t *testing.T) {
	t.Parallel()
	for _, v := range []*View{{}, {Facts: model.Facts{Transcript: model.Some(model.Transcript{})}}} {
		for _, section := range researchSections {
			if len(section.chips(v)) != 0 {
				t.Fatal("未取得か旧形式のログを表示した")
			}
		}
	}
	empty := model.Transcript{Cursor: model.TranscriptCursor{Format: model.TranscriptFormat}}
	v := &View{Facts: model.Facts{Transcript: model.Some(empty)}}
	m := Measurements(v)
	if m["turnSamples"].Value.Or(-1) != 0 || m["toolCompleted"].Value.Or(-1) != 0 || m["toolErrorRate"].Value.Present() || m["turnMean"].Value.Present() {
		t.Fatal("欠損とゼロを混同した")
	}
	for _, n := range []int{1, 2, 10, 19, 20, 99, 100} {
		tr := empty
		for range n {
			tr.Turns = append(tr.Turns, 1)
			tr.Trace.TurnTokens = append(tr.Trace.TurnTokens, 1)
		}
		tr.Observations = timedCalls(n)
		v.Facts.Transcript = model.Some(tr)
		m = Measurements(v)
		for key, known := range map[string]bool{"turnP95": n >= 20, "turnP99": n >= 100, "turnCV": n >= 2, "turnTail": n >= 20, "toolTimeP95": n >= 20, "promptP95": n >= 20, "promptCV": n >= 2, "promptTop10": n >= 10} {
			if m[key].Value.Present() != known {
				t.Errorf("%s n=%d: %+v", key, n, m[key])
			}
		}
		if n < 20 && m["turnP95"].Basis.Kind != model.BasisWaiting {
			t.Fatal("少数標本の説明なし")
		}
	}
	tr := empty
	tr.Observations = manyNames()
	tr.Turns = make([]float64, 20)
	tr.Trace.TurnTokens = make([]float64, 20)
	v.Facts.Transcript = model.Some(tr)
	m = Measurements(v)
	if m["turnCV"].Value.Present() || m["turnTail"].Value.Present() || m["promptCV"].Value.Present() || m["promptTop10"].Value.Present() || m["toolKinds"].Value.Present() || m["toolKinds"].Basis.Kind != model.BasisTooManyKinds {
		t.Fatal("ゼロ除算か追跡上限を無視した")
	}
	for _, section := range researchSections {
		_ = section.chips(v)
	}
}

func TestResearchValueUnits(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		n    float64
		unit unit
		want string
	}{{0, unitSeconds, "0.0ms"}, {0.125, unitSeconds, "125.0ms"}, {2, unitSeconds, "2s"}, {25, unitPercent, "25.0%"}, {1.25, unitRatio, "×1.25"}, {1.25, unitDecimal, "1.25"}, {1000, unitNumber, "1k"}} {
		if got := researchValue(c.n, c.unit); got != c.want {
			t.Errorf("%s=%s want %s", c.unit, got, c.want)
		}
	}
}

func TestResearchLayoutFitsConsoleWidths(t *testing.T) {
	t.Parallel()
	v := &View{Facts: model.Facts{Transcript: model.Some(researchTranscript(t))}}
	groups := Compose(v)
	for _, columns := range []int{60, 87, 130} {
		for _, line := range Layout(groups, Budget(columns)) {
			width := line.Indent
			for _, item := range line.Items {
				switch item.Sep {
				case model.SepChip:
					width += SeparatorCells
				case model.SepTight:
					width += TightSeparatorCells
				case model.SepNone:
				}
				width += Cells(item.Chip.Text())
			}
			if width > columns-3 {
				t.Errorf("幅%dで%dセルの行が切れる", columns, width)
			}
		}
	}
}
