import type { MeasurementDefinition } from "./definition.ts";

// 一次情報の概念をローカルログの記述統計へ適用する。論文の評価結果を再現するものではない。
// 分類・ラベル・単位は plugin/contracts/research-measurements.json と一致させる（research.test.ts が照合する）。
export const RESEARCH_MEASUREMENTS = {
  turnSamples: {
    label: "Samples",
    group: "Latency",
    unit: "number",
    description:
      "直近200件までの有効な完了ターン数 n",
    source: "tail",
    assessment: "neutral",
  },
  turnMean: {
    label: "Mean",
    group: "Latency",
    unit: "seconds",
    description:
      "完了ターン秒数の算術平均。未完了ターンは含まない",
    source: "tail",
    assessment: "neutral",
  },
  turnMin: {
    label: "Min",
    group: "Latency",
    unit: "seconds",
    description:
      "観測した完了ターン秒数の最小値",
    source: "tail",
    assessment: "neutral",
  },
  turnMax: {
    label: "Max",
    group: "Latency",
    unit: "seconds",
    description:
      "観測した完了ターン秒数の最大値。上限保証ではない",
    source: "tail",
    assessment: "neutral",
  },
  turnP95: {
    label: "P95",
    group: "Latency",
    unit: "seconds",
    description:
      "昇順 ceil(0.95×n) 番目。n≥20で表示",
    source: "tail",
    assessment: "neutral",
  },
  turnP99: {
    label: "P99",
    group: "Latency",
    unit: "seconds",
    description:
      "昇順 ceil(0.99×n) 番目。n≥100で表示",
    source: "tail",
    assessment: "neutral",
  },
  turnSD: {
    label: "StdDev",
    group: "Latency",
    unit: "seconds",
    description:
      "sqrt(Σ(x−平均)²/n)。標本内の母分散型標準偏差",
    source: "tail",
    assessment: "neutral",
  },
  turnCV: {
    label: "CV",
    group: "Latency",
    unit: "ratio",
    description:
      "標準偏差÷平均。n≥2かつ平均>0。品質・人間の集中力ではない",
    source: "tail",
    assessment: "neutral",
  },
  turnTail: {
    label: "Tail",
    group: "Latency",
    unit: "ratio",
    description:
      "p95÷p50。n≥20かつp50>0。観測窓内の裾の広がり",
    source: "tail",
    assessment: "neutral",
  },
  observedCalls: {
    label: "Calls",
    group: "Tools",
    unit: "number",
    description:
      "主会話でIDを確認できたツール呼び出し。直近256ID内の再掲を除外",
    source: "monitoring",
    assessment: "neutral",
  },
  toolCompleted: {
    label: "Results",
    group: "Tools",
    unit: "number",
    description:
      "呼び出しIDと対応した結果数。拒否・中断を除外",
    source: "monitoring",
    assessment: "neutral",
  },
  toolFailed: {
    label: "Errors",
    group: "Tools",
    unit: "number",
    description:
      "対応した結果のうち is_error=true の数。タスク失敗数ではない",
    source: "monitoring",
    assessment: "neutral",
  },
  toolSkipped: {
    label: "Skipped",
    group: "Tools",
    unit: "number",
    description:
      "対応した結果のうち拒否・中断した数。成否の分母に含めない",
    source: "monitoring",
    assessment: "neutral",
  },
  toolPending: {
    label: "Pending",
    group: "Tools",
    unit: "number",
    description:
      "保持中の呼び出しIDで結果がまだ対応しない数。実行中とは限らない",
    source: "monitoring",
    assessment: "neutral",
  },
  toolUnpaired: {
    label: "Unpaired",
    group: "Tools",
    unit: "number",
    description:
      "呼び出しを対応できない一意な結果。直近256結果ID内の再掲を除外",
    source: "monitoring",
    assessment: "neutral",
  },
  toolEvicted: {
    label: "Evicted",
    group: "Tools",
    unit: "number",
    description:
      "256件の上限を超えて追跡から外れた未完了ID。欠損として保持",
    source: "monitoring",
    assessment: "neutral",
  },
  toolMissingID: {
    label: "MissingID",
    group: "Tools",
    unit: "number",
    description:
      "呼び出しIDが欠けて対応付けできなかったイベント",
    source: "monitoring",
    assessment: "neutral",
  },
  toolUntimed: {
    label: "Untimed",
    group: "Tools",
    unit: "number",
    description:
      "結果は対応したが時刻欠損または逆順のため往復時間を測れない数",
    source: "monitoring",
    assessment: "neutral",
  },
  toolTimeSamples: {
    label: "Timed",
    group: "Tools",
    unit: "number",
    description:
      "直近200件までの記録時刻が対応したツール結果数 n",
    source: "monitoring",
    assessment: "neutral",
  },
  toolTimeMean: {
    label: "RoundMean",
    group: "Tools",
    unit: "seconds",
    description:
      "結果の記録時刻−呼び出しの記録時刻の平均。許可待ち・記録遅延を含み、実行時間やTTFTではない",
    source: "monitoring",
    assessment: "neutral",
  },
  toolTimeP50: {
    label: "RoundP50",
    group: "Tools",
    unit: "seconds",
    description:
      "ログ上の往復時間のnearest-rank中央値。並列呼び出しでもIDごとに対応",
    source: "monitoring",
    assessment: "neutral",
  },
  toolTimeP95: {
    label: "RoundP95",
    group: "Tools",
    unit: "seconds",
    description:
      "ログ上の往復時間のnearest-rank p95。n≥20で表示",
    source: "monitoring",
    assessment: "neutral",
  },
  toolTimeMax: {
    label: "RoundMax",
    group: "Tools",
    unit: "seconds",
    description:
      "観測したログ上の往復時間の最大値。未完了を含まない",
    source: "monitoring",
    assessment: "neutral",
  },
  toolErrorRate: {
    label: "MainErr",
    group: "Tools",
    unit: "percent",
    description:
      "is_error=true の対応済み結果数÷対応済み結果数×100。拒否・中断を除外",
    source: "wilson",
    assessment: "neutral",
  },
  toolErrorLow: {
    label: "WilsonLow",
    group: "Tools",
    unit: "percent",
    description:
      "ツールエラー率のWilson名目95%区間の下端。独立ベルヌーイ仮定は実会話では保証されない",
    source: "wilson",
    assessment: "neutral",
  },
  toolErrorHigh: {
    label: "WilsonHigh",
    group: "Tools",
    unit: "percent",
    description:
      "ツールエラー率のWilson名目95%区間の上端。エラー0件でも真の率0%とは結論しない",
    source: "wilson",
    assessment: "neutral",
  },
  toolKinds: {
    label: "Kinds",
    group: "Diversity",
    unit: "number",
    description:
      "主会話で観測したツール名の種類数。128種類上限を超えると多様性指標を非表示",
    source: "shannon",
    assessment: "neutral",
  },
  toolEntropy: {
    label: "Entropy",
    group: "Diversity",
    unit: "decimal",
    description:
      "H=−Σpᵢ log₂pᵢ（bit）。pᵢ=ツール別呼び出し数÷名前付き呼び出し数",
    source: "shannon",
    assessment: "neutral",
  },
  toolEffective: {
    label: "Effective",
    group: "Diversity",
    unit: "decimal",
    description:
      "Hill数（次数1）=2^H。均等利用に換算した有効ツール種類数",
    source: "hill",
    assessment: "neutral",
  },
  toolDominance: {
    label: "Dominant",
    group: "Diversity",
    unit: "percent",
    description:
      "最頻ツールの呼び出し数÷名前付き呼び出し数×100。高低に優劣を付けない",
    source: "shannon",
    assessment: "neutral",
  },
  toolTransitions: {
    label: "Transitions",
    group: "Diversity",
    unit: "number",
    description:
      "主会話の名前付きツール呼び出しで比較できた隣接ペア数",
    source: "shannon",
    assessment: "neutral",
  },
  toolSwitch: {
    label: "Switch",
    group: "Diversity",
    unit: "percent",
    description:
      "ツール名が変化した隣接ペア数÷比較可能な隣接ペア数×100。認知的なタスク切り替えではない",
    source: "shannon",
    assessment: "neutral",
  },
  promptSamples: {
    label: "Samples",
    group: "Budget",
    unit: "number",
    description:
      "次の人間の入力まで閉じた、トークン消費のあるプロンプト区間。直近200件。進行中区間は除外",
    source: "agents",
    assessment: "neutral",
  },
  promptMean: {
    label: "MeanTok",
    group: "Budget",
    unit: "number",
    description:
      "完了プロンプト区間の（入力+出力）トークン数の平均。入力にはキャッシュの再読込を含む",
    source: "agents",
    assessment: "neutral",
  },
  promptMedian: {
    label: "P50Tok",
    group: "Budget",
    unit: "number",
    description:
      "完了プロンプト区間のトークン数のnearest-rank中央値",
    source: "agents",
    assessment: "neutral",
  },
  promptP95: {
    label: "P95Tok",
    group: "Budget",
    unit: "number",
    description:
      "完了プロンプト区間のトークン数のnearest-rank p95。n≥20",
    source: "agents",
    assessment: "neutral",
  },
  promptMax: {
    label: "MaxTok",
    group: "Budget",
    unit: "number",
    description:
      "完了プロンプト区間の最大トークン数",
    source: "agents",
    assessment: "neutral",
  },
  promptCV: {
    label: "TokenCV",
    group: "Budget",
    unit: "ratio",
    description:
      "完了プロンプト区間のトークン数の標準偏差÷平均。n≥2。異なるタスク間の記述統計",
    source: "agents",
    assessment: "neutral",
  },
  promptTop10: {
    label: "Top10",
    group: "Budget",
    unit: "percent",
    description:
      "消費上位 ceil(0.1×n) 区間のトークン合計÷観測区間の全トークン×100。n≥10",
    source: "agents",
    assessment: "neutral",
  },
  inputPerRequest: {
    label: "InputReq",
    group: "Budget",
    unit: "number",
    description:
      "累積入力トークン÷usageを観測した応答数。キャッシュを含む。料金ではない",
    source: "agents",
    assessment: "neutral",
  },
  outputPerRequest: {
    label: "OutputReq",
    group: "Budget",
    unit: "number",
    description:
      "累積出力トークン÷usageを観測した応答数。内容の正しさを表さない",
    source: "agents",
    assessment: "neutral",
  },
  sessionCacheShare: {
    label: "Cached",
    group: "Budget",
    unit: "percent",
    description:
      "全会話のキャッシュ読込トークン÷全入力トークン×100。金額の節約率ではない",
    source: "agents",
    assessment: "neutral",
  },
  testDecided: {
    label: "TestKnown",
    group: "Evidence",
    unit: "number",
    description:
      "認識したテスト実行で合否が判定できた件数。テストケース数ではない",
    source: "swe",
    assessment: "neutral",
  },
  testUnknown: {
    label: "TestUnknown",
    group: "Evidence",
    unit: "number",
    description:
      "認識したテスト実行のうち合否不明の件数",
    source: "swe",
    assessment: "neutral",
  },
  testPassRate: {
    label: "TestPass",
    group: "Evidence",
    unit: "percent",
    description:
      "成功したテスト実行数÷合否を判定できた実行数×100。品質保証やtask success rateではない",
    source: "swe",
    assessment: "neutral",
  },
  buildUnknown: {
    label: "BuildUnknown",
    group: "Evidence",
    unit: "number",
    description:
      "認識したビルド・Lint実行のうち合否不明の件数",
    source: "swe",
    assessment: "neutral",
  },
  buildPassRate: {
    label: "BuildPass",
    group: "Evidence",
    unit: "percent",
    description:
      "成功したビルド等の実行数÷合否を判定できた実行数×100",
    source: "swe",
    assessment: "neutral",
  },
  hookErrorRate: {
    label: "HookError",
    group: "Evidence",
    unit: "percent",
    description:
      "失敗・中断したフック数÷観測したフック数×100",
    source: "swe",
    assessment: "neutral",
  },
  editErrorRate: {
    label: "EditError",
    group: "Evidence",
    unit: "percent",
    description:
      "結果がエラーだった編集呼び出し数÷編集呼び出し数×100。未到着・拒否も分母に含む記述値",
    source: "swe",
    assessment: "neutral",
  },
} as const satisfies Record<string, MeasurementDefinition>;

export type ResearchKey = keyof typeof RESEARCH_MEASUREMENTS;
