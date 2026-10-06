// 詳細ページ（#detail/<id>）の宣言データ。表示する分類・推移・比較図・空表示の文を
// ここで決め、ViewModel は経路の文字列ではなくこの定義で分岐する。
import {
  RESEARCH_MEASUREMENT_COUNT,
  RESEARCH_PANELS,
} from "./research-panels.ts";

export type ChartId = "limits" | "context";
export type DiagramSelection =
  | { readonly kind: "all" }
  | { readonly kind: "only"; readonly ids: readonly string[] };

export interface DetailDefinition {
  readonly title: string;
  readonly description: string;
  /** 表示する分類。空ならすべての分類。 */
  readonly categories: readonly string[];
  readonly charts: readonly ChartId[];
  readonly diagrams: DiagramSelection;
  /** 警告のある分類だけを表示する画面。 */
  readonly alertsOnly: boolean;
  /** 検索語が無いときに、表示できるものが無い場合の案内。 */
  readonly emptyText: string;
}
interface DetailInput {
  readonly title: string;
  readonly description: string;
  readonly categories?: readonly string[];
  readonly charts?: readonly ChartId[];
  readonly diagrams?: DiagramSelection;
  readonly alertsOnly?: boolean;
  readonly emptyText?: string;
}

export const NO_RESULTS_TEXT = "表示できる指標はありません。";
const ALL_DIAGRAMS: DiagramSelection = { kind: "all" };
const only = (...ids: readonly string[]): DiagramSelection => ({
  kind: "only",
  ids,
});
function detail({
  categories = [],
  charts = [],
  diagrams = only(),
  alertsOnly = false,
  emptyText = NO_RESULTS_TEXT,
  ...text
}: DetailInput): DetailDefinition {
  return { ...text, categories, charts, diagrams, alertsOnly, emptyText };
}

export const RESEARCH_DETAIL_ID = "research";
export const ALL_DETAIL_ID = "all";
export const RESEARCH_TITLE = `研究に基づく${RESEARCH_MEASUREMENT_COUNT}指標`;

export const DETAILS: ReadonlyMap<string, DetailDefinition> = new Map([
  [
    RESEARCH_DETAIL_ID,
    detail({
      title: RESEARCH_TITLE,
      description:
        "完了時間・ツール結果・利用の偏り・トークン消費・検証記録を観測します。数値はローカルログの記述統計で、タスク成功率や生産性の評価ではありません。各指標に算出式・観測件数・出典を表示します。",
      categories: [...new Set(RESEARCH_PANELS.map((panel) => panel.group))],
      diagrams: only(...RESEARCH_PANELS.map((panel) => panel.id)),
    }),
  ],
  ...RESEARCH_PANELS.map(
    (panel) =>
      [
        panel.id,
        detail({
          title: panel.detailTitle,
          description: panel.description,
          categories: [panel.group],
          diagrams: only(panel.id),
        }),
      ] as const,
  ),
  [
    ALL_DETAIL_ID,
    detail({
      title: "すべての指標",
      description:
        "ステータスラインと同じ計算・同じ分類で、取得できた指標をすべて表示します。",
      charts: ["limits", "context"],
      diagrams: ALL_DIAGRAMS,
    }),
  ],
  [
    "limits",
    detail({
      title: "レート制限",
      description:
        "Claude・Codex の使用率、リセット時刻、記録済みの推移。古い入力から予測は行いません。",
      categories: ["Claude", "Codex", "Forecast"],
      charts: ["limits"],
    }),
  ],
  [
    "context",
    detail({
      title: "コンテキスト",
      description:
        "使用率・残り容量と、セッションが実際に記録したトークン数の推移。",
      categories: ["Context"],
      charts: ["context"],
    }),
  ],
  [
    "alerts",
    detail({
      title: "警告と対応事項",
      description: "警告が付いている指標を、すべての分類から集約しています。",
      alertsOnly: true,
      emptyText: "取得した指標に警告はありません。",
    }),
  ],
  [
    "costs",
    detail({
      title: "コストと稼働時間",
      description:
        "セッション費用、本日の API 料金換算、消費ペースと作業時間。対象期間が異なる費用は合算しません。",
      categories: ["Cost", "Burn", "KPI"],
      diagrams: only("costs"),
    }),
  ],
  [
    "tokens",
    detail({
      title: "トークンとキャッシュ",
      description:
        "入出力、リクエスト数、キャッシュのヒット率・書き込み・再構築。",
      categories: ["Tokens", "Cache"],
      diagrams: only("tokens"),
    }),
  ],
  [
    "performance",
    detail({
      title: "性能と生産性",
      description:
        "集中率、作業量、応答時間、並列度とツールエラー。観測条件を満たさない値は未取得と表示します。",
      categories: ["KPI", "Perf", "Trace"],
      diagrams: only("performance"),
    }),
  ],
  [
    "work",
    detail({
      title: "作業とタスク",
      description: "ツール実行、編集ファイル、Todo、期限とレビュー待ち。",
      categories: ["Work", "Due", "Agent"],
      diagrams: only("work"),
    }),
  ],
  [
    "git",
    detail({
      title: "Git とプルリクエスト",
      description: "ブランチ、変更差分、未反映の変更、PR と CI の状態。",
      categories: ["Git", "PR"],
      diagrams: only("git"),
    }),
  ],
  [
    "quality",
    detail({
      title: "品質と開発習慣",
      description:
        "テスト・ビルド・Lint の実行、変更の品質、規約と開発習慣。",
      categories: ["Quality", "Rules", "Habits"],
      diagrams: only("quality"),
    }),
  ],
  [
    "agents",
    detail({
      title: "エージェントとセッション",
      description: "プロンプト、自律実行、介入、同時セッションと実行環境。",
      categories: ["Agent", "Session", "Sessions", "Trace", "Env"],
      diagrams: only("agents"),
    }),
  ],
  [
    "environment",
    detail({
      title: "端末と環境",
      description:
        "CPU の load average、空きメモリ・ディスク、バッテリーとツールのバージョン。",
      categories: ["System", "Env", "Meta", "Music"],
      diagrams: only("environment"),
    }),
  ],
]);

/** 指標1件の詳細ページは、その指標の分類だけを表示する。 */
export function metricDetail(definition: {
  readonly label: string;
  readonly description: string;
  readonly group: string;
}): DetailDefinition {
  return detail({
    title: definition.label,
    description: definition.description,
    categories: [definition.group],
  });
}
