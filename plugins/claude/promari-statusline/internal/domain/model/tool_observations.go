package model

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"
)

const (
	// ToolObservationLimit は未完了ID・重複判定の記憶上限。
	ToolObservationLimit = 256
	// ToolKindsLimit は種類別に数えるツール名の上限。超えた名前は OtherNames に数える。
	ToolKindsLimit = 128
)

// ObservedCall は主会話の呼び出しIDと記録時刻だけを保持する。引数や結果本文を保存しない。
type ObservedCall struct {
	ID string    `json:"id"`
	At time.Time `json:"at"`
}

// ToolObservations は主会話の構造化イベントから集計する。結果を呼び出しIDで対応付け、
// 拒否・中断、未到着、追跡上限による欠損をエラーから分ける。
//
// フィールドは非公開で、Called と Resulted だけが動かす。次の不変条件を常に満たす。
//
//	calls = completed + skipped + len(pending) + evicted
//	failed ≤ completed、untimed + len(seconds) ≤ completed、switches ≤ transitions
//	len(pending), len(recent), len(resultIDs) ≤ ToolObservationLimit
//	len(names) ≤ ToolKindsLimit、len(seconds) ≤ MaxTurnSamples
//
// 読むときは View を使う。保存した JSON を読み戻すときも不変条件を検査し、破れた値は受け取らない。
//
// 値として写すと map と slice の中身は写しと共有される。そのため Called と Resulted は
// 共有された中身を書き換えず、変えるたびに新しい map・slice を作る（写した側を進めても
// 元の集計は変わらない）。
type ToolObservations struct {
	calls, completed, failed, skipped     int
	unpaired, evicted, missingID, untimed int
	names                                 map[string]int
	otherNames, switches, transitions     int
	lastName                              string
	pending                               []ObservedCall
	recent, resultIDs                     []string
	seconds                               []float64
}

// ToolObservationsView は ToolObservations の読み取り専用の写し。map と slice は複製なので、
// 書き換えても集計は変わらない。
type ToolObservationsView struct {
	// Calls は主会話の一意な呼び出し。Completed は結果が届いたもの、Failed はそのうちエラー、
	// Skipped は拒否・中断で実行されなかったもの、Pending は結果待ち。
	Calls, Completed, Failed, Skipped, Pending int
	// Unpaired は対応する呼び出しの無い結果、Evicted は結果待ちの上限で追跡を外れた呼び出し、
	// MissingID はIDの無い呼び出し、Untimed は時刻が欠けて往復時間を測れない結果。
	Unpaired, Evicted, MissingID, Untimed int
	// Names は名前別の呼び出し数（上限 ToolKindsLimit 種類）、OtherNames は上限を超えた名前の呼び出し。
	Names      map[string]int
	OtherNames int
	// Transitions は隣接する名前付き呼び出しの組、Switches はそのうち名前が変わった組。
	Switches, Transitions int
	// Seconds は呼び出しから結果までのログ上の往復（秒）。直近 MaxTurnSamples 件。
	Seconds []float64
}

// View は集計の読み取り専用の写しを返す。
func (o *ToolObservations) View() ToolObservationsView {
	return ToolObservationsView{
		Calls: o.calls, Completed: o.completed, Failed: o.failed, Skipped: o.skipped, Pending: len(o.pending),
		Unpaired: o.unpaired, Evicted: o.evicted, MissingID: o.missingID, Untimed: o.untimed,
		Names: maps.Clone(o.names), OtherNames: o.otherNames,
		Switches: o.switches, Transitions: o.transitions,
		Seconds: slices.Clone(o.seconds),
	}
}

// Called は新しい主会話の呼び出しを記録する。同じIDの再掲を数えない。
func (o *ToolObservations) Called(c ToolCall) {
	if c.Side {
		return
	}
	if c.ID == "" {
		o.missingID++
		return
	}
	if slices.Contains(o.recent, c.ID) {
		return
	}
	o.recent = keepObservation(o.recent, c.ID, ToolObservationLimit)
	o.calls++
	if c.Name != "" {
		if _, known := o.names[c.Name]; known || len(o.names) < ToolKindsLimit {
			names := maps.Clone(o.names)
			if names == nil {
				names = map[string]int{}
			}
			names[c.Name]++
			o.names = names
		} else {
			o.otherNames++
		}
		if o.lastName != "" {
			o.transitions++
			if o.lastName != c.Name {
				o.switches++
			}
		}
		o.lastName = c.Name
	}
	if len(o.pending) == ToolObservationLimit {
		o.pending = slices.Clone(o.pending[1:])
		o.evicted++
	}
	o.pending = append(slices.Clip(o.pending), ObservedCall{ID: c.ID, At: c.At})
}

// Resulted は一意な結果を記録する。時間はログ上の往復であり、実行時間やTTFTではない。
func (o *ToolObservations) Resulted(r ToolResult) {
	if r.Side || r.ID == "" || slices.Contains(o.resultIDs, r.ID) {
		return
	}
	o.resultIDs = keepObservation(o.resultIDs, r.ID, ToolObservationLimit)
	i := slices.IndexFunc(o.pending, func(c ObservedCall) bool { return c.ID == r.ID })
	if i < 0 {
		o.unpaired++
		return
	}
	c := o.pending[i]
	// 容量を i に切った前半へ後半を足す: 共有された配列には書かず、空になっても nil にしない
	// （保存形式の "pending": [] を変えない）。
	o.pending = append(o.pending[:i:i], o.pending[i+1:]...)
	if r.Skipped {
		o.skipped++
		return
	}
	o.completed++
	if r.Failed {
		o.failed++
	}
	if c.At.IsZero() || r.At.IsZero() || r.At.Before(c.At) {
		o.untimed++
		return
	}
	o.seconds = keepObservation(o.seconds, r.At.Sub(c.At).Seconds(), MaxTurnSamples)
}

func keepObservation[T any](values []T, value T, limit int) []T {
	values = append(slices.Clip(values), value)
	if len(values) > limit {
		values = slices.Clone(values[len(values)-limit:])
	}
	return values
}

// toolObservationsJSON は保存形式。メンバー名は公開フィールドだった頃のまま変えない
// （既存の集計キャッシュを読めるように）。
type toolObservationsJSON struct {
	Calls       int            `json:"calls"`
	Completed   int            `json:"completed"`
	Failed      int            `json:"failed"`
	Skipped     int            `json:"skipped"`
	Unpaired    int            `json:"unpaired"`
	Evicted     int            `json:"evicted"`
	MissingID   int            `json:"missing_id"`
	Untimed     int            `json:"untimed"`
	Names       map[string]int `json:"names,omitzero"`
	OtherNames  int            `json:"other_names,omitzero"`
	Switches    int            `json:"switches"`
	Transitions int            `json:"transitions"`
	LastName    string         `json:"last_name,omitzero"`
	Pending     []ObservedCall `json:"pending,omitzero"`
	Recent      []string       `json:"recent,omitzero"`
	ResultIDs   []string       `json:"result_ids,omitzero"`
	Seconds     []float64      `json:"seconds,omitzero"`
}

// MarshalJSONTo は保存形式で書く。
func (o ToolObservations) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, &toolObservationsJSON{
		Calls: o.calls, Completed: o.completed, Failed: o.failed, Skipped: o.skipped,
		Unpaired: o.unpaired, Evicted: o.evicted, MissingID: o.missingID, Untimed: o.untimed,
		Names: o.names, OtherNames: o.otherNames, Switches: o.switches, Transitions: o.transitions,
		LastName: o.lastName, Pending: o.pending, Recent: o.recent, ResultIDs: o.resultIDs, Seconds: o.seconds,
	})
}

// UnmarshalJSONFrom は保存形式を読み、不変条件を破る値をエラーにする。読めない集計は
// 呼び出し側（集計キャッシュ）が捨てて、会話記録を先頭から読み直す。
func (o *ToolObservations) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var j toolObservationsJSON
	if err := json.UnmarshalDecode(dec, &j); err != nil {
		return err
	}
	read := ToolObservations{
		calls: j.Calls, completed: j.Completed, failed: j.Failed, skipped: j.Skipped,
		unpaired: j.Unpaired, evicted: j.Evicted, missingID: j.MissingID, untimed: j.Untimed,
		names: j.Names, otherNames: j.OtherNames, switches: j.Switches, transitions: j.Transitions,
		lastName: j.LastName, pending: j.Pending, recent: j.Recent, resultIDs: j.ResultIDs, seconds: j.Seconds,
	}
	if err := read.check(); err != nil {
		return err
	}
	*o = read
	return nil
}

// ErrBrokenObservations は不変条件を破る保存値。
var ErrBrokenObservations = errors.New("the tool observations break their invariants")

// check は不変条件を検査する。
func (o *ToolObservations) check() error {
	for _, rule := range []struct {
		holds bool
		what  string
	}{
		{min(o.calls, o.completed, o.failed, o.skipped, o.unpaired, o.evicted, o.missingID, o.untimed, o.otherNames, o.switches) >= 0, "a negative count"},
		{o.calls == o.completed+o.skipped+len(o.pending)+o.evicted, "calls ≠ completed + skipped + pending + evicted"},
		{o.failed <= o.completed, "failed > completed"},
		{o.untimed+len(o.seconds) <= o.completed, "untimed + timed > completed"},
		{o.switches <= o.transitions, "switches > transitions"},
		{max(len(o.pending), len(o.recent), len(o.resultIDs)) <= ToolObservationLimit, "more ids than are kept"},
		{len(o.names) <= ToolKindsLimit, "more tool names than are tracked"},
		{len(o.seconds) <= MaxTurnSamples, "more round trips than are kept"},
	} {
		if !rule.holds {
			return fmt.Errorf("%w: %s", ErrBrokenObservations, rule.what)
		}
	}
	return nil
}
