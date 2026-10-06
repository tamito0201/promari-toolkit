package dashboard

import (
	"path/filepath"
	"strconv"
	"time"

	"promari-statusline/internal/application/usecase"
	"promari-statusline/internal/domain/model"
)

// The JSON of /api/snapshot. It is the contract with web/src/api.ts: a member
// renamed here is renamed there. A value that is not known is left out, never
// written as zero.
type snapshotDTO struct {
	Version        string                    `json:"version"`
	At             time.Time                 `json:"at"`
	InputAt        time.Time                 `json:"inputAt,omitzero"`
	Live           bool                      `json:"live"`
	Session        sessionDTO                `json:"session"`
	Headline       headlineDTO               `json:"headline"`
	Bands          []bandDTO                 `json:"bands"`
	Groups         []groupDTO                `json:"groups"`
	RateHistory    []ratePointDTO            `json:"rateHistory"`
	CodexHistory   []codexPointDTO           `json:"codexHistory"`
	ContextHistory []contextPointDTO         `json:"contextHistory"`
	LimitsAt       time.Time                 `json:"limitsAt,omitzero"`
	Measurements   map[string]measurementDTO `json:"measurements"`
}

// 集計ゼロは value: 0、算出不能は値を省略して理由を返す。
type measurementDTO struct {
	Value *float64 `json:"value,omitzero"`
	Note  string   `json:"note,omitzero"`
}

// 履歴の時刻は記録時刻のまま返す。欠測をゼロで補完しない。
type ratePointDTO struct {
	At       time.Time `json:"at"`
	FiveHour *float64  `json:"fiveHour,omitzero"`
	SevenDay *float64  `json:"sevenDay,omitzero"`
}

type contextPointDTO struct {
	At     time.Time `json:"at"`
	Tokens float64   `json:"tokens"`
}

type codexPointDTO struct {
	At        time.Time       `json:"at"`
	Primary   *codexWindowDTO `json:"primary,omitzero"`
	Secondary *codexWindowDTO `json:"secondary,omitzero"`
}

type codexWindowDTO struct {
	UsedPct       float64 `json:"usedPct"`
	WindowMinutes float64 `json:"windowMinutes"`
}

type sessionDTO struct {
	Name    string `json:"name,omitzero"`
	Model   string `json:"model,omitzero"`
	Project string `json:"project,omitzero"`
	Version string `json:"version,omitzero"`
}

type windowDTO struct {
	UsedPct  float64   `json:"usedPct"`
	ResetsAt time.Time `json:"resetsAt,omitzero"`
}

type headlineDTO struct {
	ContextPct *float64   `json:"contextPct,omitzero"`
	FiveHour   *windowDTO `json:"fiveHour,omitzero"`
	SevenDay   *windowDTO `json:"sevenDay,omitzero"`
	SessionUSD *float64   `json:"sessionUsd,omitzero"`
	TodayUSD   *float64   `json:"todayUsd,omitzero"`
	Running    int        `json:"running"`
}

type bandDTO struct {
	Band     int    `json:"band"`
	Name     string `json:"name"`
	Question string `json:"question"`
}

type groupDTO struct {
	Band  int       `json:"band"`
	Title string    `json:"title"`
	Tone  string    `json:"tone"`
	Chips []chipDTO `json:"chips"`
}

type chipDTO struct {
	Spans []spanDTO `json:"spans"`
}

type spanDTO struct {
	Text  string `json:"text"`
	Tone  string `json:"tone,omitzero"`
	Bold  bool   `json:"bold,omitzero"`
	Alarm bool   `json:"alarm,omitzero"`
}

// toneName names a tone for the page. The page maps a name to a colour, as the
// terminal presenter maps a tone to an SGR sequence. A tone without a name of
// its own (TonePlain, or one the page does not know) is "".
func toneName(t model.Tone) string {
	var name string
	switch t {
	case model.ToneAccent:
		name = "accent"
	case model.ToneGood:
		name = "good"
	case model.ToneCaution:
		name = "caution"
	case model.ToneDanger:
		name = "danger"
	case model.ToneInfo:
		name = "info"
	case model.ToneMoney:
		name = "money"
	case model.ToneNote:
		name = "note"
	case model.ToneMuted:
		name = "muted"
	case model.ToneBrand:
		name = "brand"
	case model.TonePlain:
	}
	return name
}

// note words what a measurement rests on, or why it has no value. A plain
// count needs no note.
func note(b model.Basis) string {
	var text string
	switch b.Kind {
	case model.BasisObserved:
		text = scope(b.Scope, b.Window) + " · n=" + strconv.Itoa(b.N)
	case model.BasisWaiting:
		text = "観測 " + strconv.Itoa(b.N) + " / 必要 " + strconv.Itoa(b.Required) + " 件"
	case model.BasisNoBlock:
		text = "稼働枠なし"
	case model.BasisTooManyKinds:
		text = "ツール種類数が追跡上限" + strconv.Itoa(b.Limit) + "を超過"
	case model.BasisCounted:
	}
	return text
}

// scope words the observations a value was computed from; window is the
// bound of the observations kept.
func scope(s model.Scope, window int) string {
	w := strconv.Itoa(window)
	var text string
	switch s {
	case model.ScopeRecentTurns:
		text = "直近" + w + "完了ターン以内"
	case model.ScopeTurnSpread:
		text = "完了ターン内のばらつき"
	case model.ScopeTailRatio:
		text = "同一観測窓のp95/p50"
	case model.ScopeNearestRank:
		text = "直近" + w + "観測以内・nearest-rank"
	case model.ScopePairedResults:
		text = "主会話の対応済み結果・拒否/中断を除外"
	case model.ScopeRoundTrips:
		text = "主会話のログ往復・直近" + w + "件以内"
	case model.ScopeAdjacentCalls:
		text = "隣接する主会話の名前付き呼び出し"
	case model.ScopeNamedCalls:
		text = "主会話の名前付きツール呼び出し・優劣を表さない"
	case model.ScopeClosedPrompts:
		text = "次の入力で閉じた消費区間・直近" + w + "件以内"
	case model.ScopePromptSpread:
		text = "異なるプロンプトの記述統計"
	case model.ScopeTopShare:
		text = "進行中の区間を除外・上位10%は切り上げ"
	case model.ScopeUsageResponses:
		text = "usageを観測した会話内の応答"
	case model.ScopeAllInput:
		text = "全入力トークンが分母。金額の削減率ではない"
	case model.ScopeRecognizedRuns:
		text = "認識できた実行イベント・タスク成功率ではない"
	case model.ScopeNone:
	}
	return text
}

func toDTO(s *usecase.Snapshot, version string) snapshotDTO {
	project := s.Session.ProjectDir
	if project == "" {
		project = s.Session.Dir
	}
	if project != "" {
		project = filepath.Base(project)
	}
	out := snapshotDTO{
		Version: version,
		At:      s.At,
		InputAt: s.InputAt,
		Live:    s.Live,
		Session: sessionDTO{Name: s.Session.Name, Model: s.Session.Model, Project: project, Version: s.Session.Version},
		Headline: headlineDTO{
			ContextPct: ptr(s.Headline.ContextPct),
			FiveHour:   window(s.Headline.FiveHour),
			SevenDay:   window(s.Headline.SevenDay),
			SessionUSD: ptr(s.Headline.SessionUSD),
			TodayUSD:   ptr(s.Headline.TodayUSD),
			Running:    s.Headline.Running,
		},
		Bands:          make([]bandDTO, 0, len(s.Bands)),
		Groups:         make([]groupDTO, 0, len(s.Groups)),
		RateHistory:    make([]ratePointDTO, 0, len(s.RateHistory)),
		CodexHistory:   make([]codexPointDTO, 0, len(s.CodexHistory)),
		ContextHistory: make([]contextPointDTO, 0, len(s.ContextHistory)),
		LimitsAt:       s.LimitsAt,
		Measurements:   make(map[string]measurementDTO, len(s.Measurements)),
	}
	for key, m := range s.Measurements {
		out.Measurements[key] = measurementDTO{Value: ptr(m.Value), Note: note(m.Basis)}
	}
	for _, p := range s.RateHistory {
		out.RateHistory = append(out.RateHistory, ratePointDTO{At: p.At, FiveHour: ptr(p.FiveHour), SevenDay: ptr(p.SevenDay)})
	}
	for _, p := range s.CodexHistory {
		out.CodexHistory = append(out.CodexHistory, codexPointDTO{At: p.At, Primary: codexWindow(p.Primary), Secondary: codexWindow(p.Secondary)})
	}
	for _, p := range s.ContextHistory {
		out.ContextHistory = append(out.ContextHistory, contextPointDTO{At: p.At, Tokens: p.Tokens})
	}
	for _, b := range s.Bands {
		out.Bands = append(out.Bands, bandDTO{Band: int(b.Band), Name: b.Name, Question: b.Question})
	}
	for _, g := range s.Groups {
		gd := groupDTO{Band: int(g.Band), Title: g.Title, Tone: toneName(g.Tone), Chips: make([]chipDTO, 0, len(g.Chips))}
		for _, c := range g.Chips {
			cd := chipDTO{Spans: make([]spanDTO, 0, len(c))}
			for _, sp := range c {
				cd.Spans = append(cd.Spans, spanDTO{Text: sp.Text, Tone: toneName(sp.Tone), Bold: sp.Bold, Alarm: sp.Alarm})
			}
			gd.Chips = append(gd.Chips, cd)
		}
		out.Groups = append(out.Groups, gd)
	}
	return out
}

// ptr returns the value of an optional, or nil when it is absent.
func ptr[T any](o model.Optional[T]) *T {
	if v, ok := o.Get(); ok {
		return &v
	}
	return nil
}

func window(o model.Optional[model.RateWindow]) *windowDTO {
	if w, ok := o.Get(); ok {
		return &windowDTO{UsedPct: w.UsedPct, ResetsAt: w.ResetsAt}
	}
	return nil
}

func codexWindow(o model.Optional[model.CodexWindow]) *codexWindowDTO {
	if w, ok := o.Get(); ok {
		return &codexWindowDTO{UsedPct: w.UsedPct, WindowMinutes: w.WindowMinutes}
	}
	return nil
}
