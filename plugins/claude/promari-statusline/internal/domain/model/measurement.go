package model

// Basis は集計値の根拠: どの観測から算出したか、または表示しない理由。表示文は持たず、
// 文言は表示側が組み立てる。ゼロ値（BasisCounted）は根拠の注記が要らない単純な集計値。
type Basis struct {
	Kind BasisKind
	// Scope は BasisObserved の観測範囲。
	Scope Scope
	// N は観測数。BasisObserved では算出に使った件数、BasisWaiting ではこれまでの件数。
	N int
	// Window は Scope が名指す観測窓の上限件数（窓を持たない Scope では 0）。
	Window int
	// Required は BasisWaiting で表示に必要な観測数。
	Required int
	// Limit は BasisTooManyKinds で超えた追跡上限。
	Limit int
}

// BasisKind は根拠の種類。
type BasisKind uint8

// 根拠の種類。
const (
	// BasisCounted は注記の要らない集計値。
	BasisCounted BasisKind = iota
	// BasisObserved は Scope の観測 N 件から算出した値。
	BasisObserved
	// BasisWaiting は観測が Required 件に満たず、まだ表示しない値。
	BasisWaiting
	// BasisNoBlock は稼働中の課金枠が無く、算出できない値。
	BasisNoBlock
	// BasisTooManyKinds はツールの種類数が追跡上限 Limit を超え、算出できない値。
	BasisTooManyKinds
)

// Scope は観測値の母集団。値の読み方（何を含み何を含まないか）を表す。
type Scope uint8

// 観測範囲。
const (
	ScopeNone Scope = iota
	// ScopeRecentTurns は直近 Window 件以内の完了ターン。
	ScopeRecentTurns
	// ScopeTurnSpread は完了ターン内のばらつき。
	ScopeTurnSpread
	// ScopeTailRatio は同一観測窓の p95/p50。
	ScopeTailRatio
	// ScopeNearestRank は直近 Window 件以内の観測の nearest-rank 分位点。
	ScopeNearestRank
	// ScopePairedResults は主会話の対応済みの結果（拒否・中断を除く）。
	ScopePairedResults
	// ScopeRoundTrips は主会話のログ上の往復、直近 Window 件以内。
	ScopeRoundTrips
	// ScopeAdjacentCalls は隣接する主会話の名前付き呼び出し。
	ScopeAdjacentCalls
	// ScopeNamedCalls は主会話の名前付きツール呼び出し（優劣を表さない）。
	ScopeNamedCalls
	// ScopeClosedPrompts は次の入力で閉じた消費区間、直近 Window 件以内。
	ScopeClosedPrompts
	// ScopePromptSpread は異なるプロンプトの記述統計。
	ScopePromptSpread
	// ScopeTopShare は進行中の区間を除いた上位10%（切り上げ）の占有率。
	ScopeTopShare
	// ScopeUsageResponses は usage を観測した会話内の応答。
	ScopeUsageResponses
	// ScopeAllInput は全入力トークンが分母の割合（金額の削減率ではない）。
	ScopeAllInput
	// ScopeRecognizedRuns は認識できた実行イベント（タスク成功率ではない）。
	ScopeRecognizedRuns
)
