// 研究指標の画面は宣言データだけで決める。パネルの追加はこの配列に1件足すだけで、
// ダッシュボードの帯・詳細ページ・経路・件数表示は ViewModel がここから導出する。
import { RESEARCH_MEASUREMENTS, type ResearchKey } from "./research.ts";
import type { MeasurementKey } from "./measurements.ts";

/** 1つの図の宣言。形ごとの描き方は ViewModel が持ち、ここは割り当てだけを持つ。 */
export interface ResearchVisual {
  /**
   * 図の形。range＝共通目盛の分布点、composition＝全体を1とした構成比、
   * rings＝割合のリング、columns＝件数の柱、lollipop＝点つきの棒、bars＝棒。
   */
  readonly form:
    | "range"
    | "composition"
    | "rings"
    | "columns"
    | "lollipop"
    | "bars";
  readonly keys: readonly MeasurementKey[];
  readonly title: string;
  readonly note: string;
  /** bars だけが使う。単位の違う組を同じ目盛へ載せない。 */
  readonly sharedScale?: boolean;
}

export interface ResearchPanelDefinition {
  /** 詳細ページの識別子（#detail/<id>）。 */
  readonly id: string;
  readonly number: string;
  readonly title: string;
  readonly meta: string;
  readonly detailTitle: string;
  readonly description: string;
  /** 詳細ページで表示する指標の分類。 */
  readonly group: string;
  /** 1枚目のグリッドで広い枠を使う（図が多いパネル）。 */
  readonly wide?: boolean;
  /** 図の割り当て。単位と母数の揃う組だけを同じ図で比べる。 */
  readonly visuals: readonly ResearchVisual[];
  /** どの図にも出ない残り全部。群の全指標から図の分を除いて導出し、手で列挙しない。 */
  readonly readings: readonly ResearchKey[];
}

/** readings を導出する前のパネル定義。 */
type PanelSeed = Omit<ResearchPanelDefinition, "readings">;

const PANEL_SEEDS = [
  {
    id: "latency",
    number: "12",
    title: "完了時間の分布",
    meta: "LATENCY · OBSERVED",
    detailTitle: "完了時間の分布",
    description:
      "直近200完了ターン以内。p95は20件、p99は100件から表示。未完了は標本に入らず、観測された裾にも上限保証はありません。",
    group: "Latency",
    visuals: [
      {
        form: "range",
        keys: ["turnMin", "turnP50", "turnMean", "turnP95", "turnP99", "turnMax"],
        title: "Duration",
        note: "完了ターン・直近200件以内 · 共通目盛の分布点",
      },
      {
        form: "rings",
        keys: ["turnCV", "turnTail"],
        title: "Spread",
        note: "CV＝SD÷Mean · Tail＝P95÷P50。ばらつきの記述で、良し悪しの判定ではありません",
      },
    ],
  },
  {
    id: "tooling",
    wide: true,
    number: "13",
    title: "ツール結果と欠損",
    meta: "TOOL OUTCOMES",
    detailTitle: "ツール結果と観測の欠損",
    description:
      "主会話の呼び出しと結果をIDで対応付けます。エラーと拒否・中断・未到着を分離。往復時間は記録時刻の差であり、ツール本体の実行時間ではありません。",
    group: "Tools",
    visuals: [
      {
        form: "composition",
        keys: [
          "toolCompleted",
          "toolFailed",
          "toolSkipped",
          "toolPending",
          "toolUnpaired",
        ],
        title: "Outcomes",
        note: "呼び出しの行き先の構成比。ErrorsはResultsの内数",
      },
      {
        form: "range",
        keys: ["toolTimeP50", "toolTimeMean", "toolTimeP95", "toolTimeMax"],
        title: "Round trip",
        note: "記録時刻の差 · ツール本体の実行時間ではありません",
      },
      {
        form: "lollipop",
        keys: ["toolErrorLow", "toolErrorRate", "toolErrorHigh"],
        title: "Error rate",
        note: "Wilson 95%区間の下限・実測・上限",
      },
    ],
  },
  {
    id: "diversity",
    number: "14",
    title: "ツール利用の偏り",
    meta: "DIVERSITY · NO SCORE",
    detailTitle: "ツール利用の偏り",
    description:
      "Shannon entropyとHill数をツール利用頻度へ適用します。探索範囲や利用の偏りを記述し、高い値が良いとは判定しません。",
    group: "Diversity",
    visuals: [
      {
        form: "bars",
        keys: ["toolKinds", "toolEffective"],
        title: "Tool diversity",
        note: "実際の種類数と均等利用に換算した有効種類数",
        sharedScale: false,
      },
      {
        form: "rings",
        keys: ["toolDominance", "toolSwitch"],
        title: "Concentration",
        note: "最頻ツールの占有率と、隣り合う呼び出しが替わる割合",
      },
    ],
  },
  {
    id: "budget",
    wide: true,
    number: "15",
    title: "トークン消費の分布",
    meta: "TOKEN DISTRIBUTION",
    detailTitle: "トークン消費の分布",
    description:
      "次の入力で閉じた消費区間を比較します。入力にはキャッシュ読込を含みます。トークン数と料金を区別し、進行中の区間を除外します。",
    group: "Budget",
    visuals: [
      {
        form: "range",
        keys: ["promptMedian", "promptMean", "promptP95", "promptMax"],
        title: "Tokens",
        note: "進行中の区間を除外・直近200区間以内",
      },
      {
        form: "composition",
        keys: ["inputPerRequest", "outputPerRequest"],
        title: "Per request",
        note: "1リクエスト平均の入出力の構成比。入力にはキャッシュ読込を含む",
      },
      {
        form: "rings",
        keys: ["sessionCacheShare", "promptTop10"],
        title: "Shares",
        note: "入力に占めるキャッシュ読込と、上位10%区間が占める消費",
      },
    ],
  },
  {
    id: "evidence",
    number: "16",
    title: "検証結果の観測",
    meta: "CHECK EVIDENCE",
    detailTitle: "検証記録と不明な結果",
    description:
      "認識できたテスト・ビルドの実行結果を数えます。テストケース数、コードカバレッジ、タスク成功率はこのログから推定しません。",
    group: "Evidence",
    visuals: [
      {
        form: "columns",
        keys: ["testDecided", "testUnknown", "buildUnknown"],
        title: "Checks",
        note: "認識した実行の結果。ケース数・品質スコアではありません",
      },
      {
        form: "rings",
        keys: [
          "testPassRate",
          "buildPassRate",
          "hookErrorRate",
          "editErrorRate",
        ],
        title: "Rates",
        note: "結果の分かった実行だけを分母にした割合",
      },
    ],
  },
] as const satisfies readonly PanelSeed[];

const RESEARCH_KEYS = Object.keys(RESEARCH_MEASUREMENTS) as ResearchKey[];

/** 群の全指標のうち、どの図にも出ない残り。宣言順を保つ。 */
function remainingReadings(seed: PanelSeed): readonly ResearchKey[] {
  const drawn: ReadonlySet<MeasurementKey> = new Set(
    seed.visuals.flatMap((visual) => visual.keys),
  );
  return RESEARCH_KEYS.filter(
    (key) =>
      RESEARCH_MEASUREMENTS[key].group === seed.group && !drawn.has(key),
  );
}

// 1パネル＝1群。群の全指標は、図（値つき）か readings のどちらかに必ず出る。
export const RESEARCH_PANELS: readonly ResearchPanelDefinition[] =
  PANEL_SEEDS.map((seed) => ({ ...seed, readings: remainingReadings(seed) }));

/** 研究指標の件数。表示文へ数を直書きせず、定義から数える。 */
export const RESEARCH_MEASUREMENT_COUNT = RESEARCH_KEYS.length;
