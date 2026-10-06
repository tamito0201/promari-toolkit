// 研究指標の画面は宣言データだけで決める。パネルの追加はこの配列に1件足すだけで、
// 詳細ページ・経路・概観の件数表示は ViewModel がここから導出する。
import { RESEARCH_MEASUREMENTS, type ResearchKey } from "./research.ts";
import type { MeasurementKey } from "./measurements.ts";

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
  /** 単位と母数の揃う組だけを同じ図で比べる。 */
  readonly comparison: {
    readonly keys: readonly MeasurementKey[];
    readonly title: string;
    readonly note: string;
    readonly sharedScale: boolean;
  };
  readonly readings: readonly ResearchKey[];
}

export const RESEARCH_PANELS = [
  {
    id: "latency",
    number: "12",
    title: "完了時間の分布",
    meta: "LATENCY · OBSERVED",
    detailTitle: "完了時間の分布",
    description:
      "直近200完了ターン以内。p95は20件、p99は100件から表示。未完了は標本に入らず、観測された裾にも上限保証はありません。",
    group: "Latency",
    comparison: {
      keys: ["turnMean", "turnP50", "turnP95", "turnP99", "turnMax"],
      title: "Duration",
      note: "完了ターン・直近200件以内",
      sharedScale: true,
    },
    readings: ["turnSamples", "turnCV", "turnTail"],
  },
  {
    id: "tooling",
    number: "13",
    title: "ツール結果と欠損",
    meta: "TOOL OUTCOMES",
    detailTitle: "ツール結果と観測の欠損",
    description:
      "主会話の呼び出しと結果をIDで対応付けます。エラーと拒否・中断・未到着を分離。往復時間は記録時刻の差であり、ツール本体の実行時間ではありません。",
    group: "Tools",
    comparison: {
      keys: [
        "toolCompleted",
        "toolFailed",
        "toolSkipped",
        "toolPending",
        "toolUnpaired",
      ],
      title: "Events",
      note: "ErrorsはResultsの内数。棒を合算しません",
      sharedScale: true,
    },
    readings: ["toolErrorRate", "toolErrorLow", "toolErrorHigh"],
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
    comparison: {
      keys: ["toolKinds", "toolEffective"],
      title: "Tool diversity",
      note: "実際の種類数と均等利用に換算した有効種類数",
      sharedScale: false,
    },
    readings: ["toolEntropy", "toolDominance", "toolSwitch"],
  },
  {
    id: "budget",
    number: "15",
    title: "トークン消費の分布",
    meta: "TOKEN DISTRIBUTION",
    detailTitle: "トークン消費の分布",
    description:
      "次の入力で閉じた消費区間を比較します。入力にはキャッシュ読込を含みます。トークン数と料金を区別し、進行中の区間を除外します。",
    group: "Budget",
    comparison: {
      keys: ["promptMean", "promptMedian", "promptP95", "promptMax"],
      title: "Tokens",
      note: "進行中の区間を除外・直近200区間以内",
      sharedScale: true,
    },
    readings: ["promptSamples", "promptCV", "promptTop10"],
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
    comparison: {
      keys: ["testDecided", "testUnknown", "buildUnknown"],
      title: "Checks",
      note: "認識した実行の結果。ケース数・品質スコアではありません",
      sharedScale: true,
    },
    readings: ["testPassRate", "buildPassRate", "hookErrorRate"],
  },
] as const satisfies readonly ResearchPanelDefinition[];

/** 研究指標の件数。表示文へ数を直書きせず、定義から数える。 */
export const RESEARCH_MEASUREMENT_COUNT = Object.keys(
  RESEARCH_MEASUREMENTS,
).length;
