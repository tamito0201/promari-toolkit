import type { Snapshot, Tone } from "./model.ts";
import {
  activityColumns,
  qualityTiles,
  timeAllocation,
  rateHeatmap,
  type HeatmapVM,
  instrument,
  percentageInstrument,
  compareMetrics,
  rings,
  tokenComposition,
  gitBalance,
  type InstrumentVM,
  type VisualVM,
} from "./visualization.ts";
import {
  sections,
  alarmed,
  anchor,
  pieces,
  signalValue,
  signalText,
  hasAlarm,
  type Category,
  type Section,
  type Piece,
} from "./categories.ts";
import { duration, usd, clock, dateTime } from "./format.ts";
import { historyPoints, codexSeries, type Point } from "./series.ts";
import { parseRoute } from "./routes.ts";
import {
  MEASUREMENTS,
  measurementNumber,
  measurementText,
  measurementSource,
  type MeasurementKey,
} from "./measurements.ts";
import { DETAILS, metricDetail } from "./detail-catalog.ts";
import { RESEARCH_PANELS as RESEARCH_PANEL_DEFINITIONS } from "./research-panels.ts";

// ViewModel の内部状態。ここから表示用の値を純粋関数で導出する。
export interface DashboardState {
  readonly snapshot: Snapshot | undefined;
  readonly hash: string;
  readonly rangeMinutes: number;
  readonly query: string;
  readonly onlyAlerts: boolean;
  readonly paused: boolean;
  readonly busy: boolean;
  readonly offline: boolean;
  readonly visible: boolean;
  readonly fullscreen: boolean;
  readonly fullscreenFailed: boolean;
  readonly hiddenSeries?: readonly string[];
  readonly focusedPanel?: string | undefined;
  readonly revision?: number;
}

export interface MetricVM {
  readonly label: string;
  readonly value: string;
  readonly known: boolean;
  readonly tone: Tone;
  readonly explanation: string;
  readonly source?:
    | { readonly label: string; readonly href: string }
    | undefined;
  readonly key?: MeasurementKey;
  readonly visual?: InstrumentVM;
}
export interface LinkVM {
  readonly label: string;
  readonly href: string;
  readonly alarm: boolean;
}
export interface StatVM {
  readonly label: string;
  readonly value: string;
  readonly note: string;
  readonly href: string;
  readonly ratio: number | undefined;
  readonly featured: boolean;
  readonly gauge?: InstrumentVM;
}
export interface SeriesVM {
  readonly name: string;
  readonly color: "mint" | "sky" | "teal" | "codex" | "codexOther";
  readonly points: readonly Point[];
  readonly hidden?: boolean;
}
export type BlockVM =
  | {
      readonly kind: "metrics";
      readonly items: readonly MetricVM[];
      readonly two: boolean;
    }
  | {
      readonly kind: "text";
      readonly text: string;
      readonly style: "panel-foot" | "branch";
    }
  | {
      readonly kind: "alerts";
      readonly count: number;
      readonly items: readonly (LinkVM & { readonly text: string })[];
      readonly monitors: readonly (LinkVM & {
        readonly state: string;
        readonly stale: boolean;
      })[];
    }
  | ChartVM
  | VisualVM;
export interface ChartVM {
  readonly kind: "chart";
  readonly title: string;
  readonly series: readonly SeriesVM[];
  readonly from: number;
  readonly until: number;
  readonly unit: string;
  readonly maximum: number | undefined;
  readonly heatmap?: HeatmapVM;
  readonly route: string;
}
export interface PanelVM {
  readonly title: string;
  readonly meta: string;
  readonly kind: string;
  readonly route: string | undefined;
  readonly number: string;
  readonly blocks: readonly BlockVM[];
}
export interface CardVM {
  readonly id: string;
  readonly title: string;
  readonly alarm: boolean;
  readonly state: string;
  readonly chips: readonly {
    readonly alarm: boolean;
    readonly pieces: readonly Piece[];
  }[];
}
export interface DetailVM {
  readonly title: string;
  readonly description: string;
  readonly measurements: readonly MetricVM[];
  readonly bands: readonly {
    readonly name: string;
    readonly question: string;
    readonly cards: readonly CardVM[];
  }[];
  readonly charts: readonly PanelVM[];
  readonly emptyText: string;
}
export interface PreviewVM {
  readonly route: string;
  readonly panel: PanelVM;
  readonly detail: DetailVM;
  readonly freshness: string;
}
export interface DashboardVM {
  readonly revision: number;
  readonly preview: PreviewVM | undefined;
  readonly hash: string;
  readonly overview: boolean;
  readonly title: string;
  readonly breadcrumb: string;
  readonly status: { readonly kind: string; readonly text: string };
  readonly freshness: string;
  readonly freshnessClass: string;
  readonly detailFreshness: string;
  readonly updated: string;
  readonly version: string;
  readonly controls: {
    readonly busy: boolean;
    readonly paused: boolean;
    readonly pauseText: string;
    readonly query: string;
    readonly onlyAlerts: boolean;
    readonly rangeMinutes: number;
    readonly fullscreenLabel: string;
    readonly fullscreenTitle: string;
  };
  readonly summary: readonly StatVM[];
  readonly panels: readonly PanelVM[];
  readonly categories: readonly LinkVM[];
  readonly detail: DetailVM;
}

const categoryLink = (c: Category): LinkVM => ({
  label: c.name,
  href: `#category/${encodeURIComponent(c.name)}`,
  alarm: hasAlarm(c),
});
const pct = (value: number | undefined): string =>
  value === undefined ? "—" : `${Math.round(value)}%`;
const metrics = (items: MetricVM[]): BlockVM => ({
  kind: "metrics",
  items,
  two: false,
});

function reading(s: Snapshot, key: MeasurementKey, tone: Tone = ""): MetricVM {
  return {
    label: MEASUREMENTS[key].label,
    value: measurementText(s, key),
    known: measurementNumber(s, key) !== undefined,
    explanation: [MEASUREMENTS[key].description, s.measurements?.[key]?.note]
      .filter(Boolean)
      .join("。"),
    source: measurementSource(key),
    key,
    visual: instrument(s, key),
    tone,
  };
}
function panel(
  title: string,
  meta: string,
  route: string,
  number: string,
  blocks: BlockVM[],
  kind = "small-panel",
): PanelVM {
  return { title, meta, route, number, blocks, kind };
}

function summary(s: Snapshot, cats: readonly Category[]): StatVM[] {
  const h = s.headline;
  const reset = (at: string | undefined): string =>
    at
      ? `リセットまで ${duration(Date.parse(at) - Date.parse(s.at))}`
      : "リセット時刻は未取得";
  const codexLabel = signalValue(cats, "Codex", /(?:^|\s)(\d+[dhm])\s+[█░▓▒]/u);
  const codex = signalValue(
    cats,
    "Codex",
    /(?:^|\s)\d+[dhm]\s+[█░▓▒\s]*([\d.]+)%/u,
  );
  const burn =
    measurementNumber(s, "burnRate") !== undefined ? "burnRate" : "sessionBurn";
  const stat = (
    label: string,
    value: string,
    note: string,
    route: string,
    ratio?: number,
    featured = false,
  ): StatVM => ({
    label,
    value,
    note,
    href: `#detail/${route}`,
    ratio,
    featured,
    ...(["Context", "Claude · 5h", "Claude · 7d"].includes(label) ||
    label.startsWith("Codex")
      ? { gauge: percentageInstrument(label, label, ratio) }
      : {}),
  });
  return [
    stat(
      "Context",
      pct(h.contextPct),
      "コンテキスト使用率",
      "context",
      h.contextPct,
    ),
    stat(
      "Claude · 5h",
      pct(h.fiveHour?.usedPct),
      reset(h.fiveHour?.resetsAt),
      "limits",
      h.fiveHour?.usedPct,
    ),
    stat(
      "Claude · 7d",
      pct(h.sevenDay?.usedPct),
      reset(h.sevenDay?.resetsAt),
      "limits",
      h.sevenDay?.usedPct,
    ),
    stat(
      codexLabel ? `Codex · ${codexLabel}` : "Codex",
      codex === undefined ? "—" : `${codex}%`,
      "Codex の記録済み使用率",
      "limits",
      codex === undefined ? undefined : Number(codex),
    ),
    stat(
      "Today cost",
      h.todayUsd === undefined ? "—" : usd(h.todayUsd),
      "本日 · API 料金換算",
      "costs",
      undefined,
      true,
    ),
    stat(
      "Session cost",
      h.sessionUsd === undefined ? "—" : usd(h.sessionUsd),
      "このセッションの費用",
      "costs",
    ),
    stat(
      MEASUREMENTS[burn].label,
      measurementText(s, burn),
      burn === "burnRate"
        ? "1時間あたり · ccusage"
        : "1時間あたり · セッション平均",
      "costs",
    ),
    stat(
      "Running sessions",
      String(h.running),
      "このマシンのセッション",
      "agents",
    ),
  ];
}

function charts(
  s: Snapshot,
  range: number,
  detail = false,
  hiddenSeries: readonly string[] = [],
): [PanelVM, PanelVM] {
  const until = Date.parse(s.at),
    from = until - range * 60_000;
  const build = (
    title: string,
    series: SeriesVM[],
    route: string,
    unit: string,
    maximum?: number,
  ): PanelVM => ({
    title,
    meta: `過去 ${range < 60 ? `${range} 分` : `${range / 60} 時間`}`,
    route: detail ? undefined : route,
    number: route === "limits" ? "01" : "02",
    kind: `chart-panel ${route === "limits" ? "rate-panel" : "context-panel"}`,
    blocks: [
      {
        kind: "chart",
        title,
        from,
        until,
        unit,
        maximum,
        route,
        ...(route === "limits"
          ? {
              heatmap: rateHeatmap(
                series.map((item) => ({
                  ...item,
                  hidden: hiddenSeries.includes(item.name),
                })),
                from,
                until,
              ),
            }
          : {}),
        series: series.map((series) => ({
          ...series,
          hidden: hiddenSeries.includes(series.name),
          points: historyPoints([...series.points], from, until),
        })),
      },
    ],
  });
  const rates = s.rateHistory ?? [];
  return [
    build(
      "レート制限の推移",
      [
        {
          name: "Claude · 5h",
          color: "mint",
          points: rates.map((p) => ({
            at: Date.parse(p.at),
            value: p.fiveHour ?? NaN,
          })),
        },
        {
          name: "Claude · 7d",
          color: "sky",
          points: rates.map((p) => ({
            at: Date.parse(p.at),
            value: p.sevenDay ?? NaN,
          })),
        },
        ...codexSeries(s.codexHistory ?? []).map(
          (series, i): SeriesVM => ({
            ...series,
            color: i === 0 ? "codex" : "codexOther",
          }),
        ),
      ],
      "limits",
      "%",
      100,
    ),
    build(
      "コンテキストの推移",
      [
        {
          name: "Context tokens",
          color: "teal",
          points: (s.contextHistory ?? []).map((p) => ({
            at: Date.parse(p.at),
            value: p.tokens,
          })),
        },
      ],
      "context",
      " tokens",
    ),
  ];
}

interface PanelContext {
  s: Snapshot;
  cats: readonly Category[];
  all: readonly Section[];
}
type PanelProjector = (context: PanelContext) => PanelVM;

// パネルの追加はこの定義に閉じ、View と更新制御を変更しなくてよい。
const PANELS: readonly PanelProjector[] = [
  ({ s, cats, all }) => {
    const alarms = alarmed(all);
    return panel(
      "警告・対応事項",
      "ACTION REQUIRED",
      "alerts",
      "03",
      [
        {
          kind: "alerts",
          count: alarms.length,
          items: alarms.slice(0, 3).map(({ category: c }) => ({
            ...categoryLink(c),
            text: c.chips
              .filter((chip) => chip.spans.some((sp) => sp.alarm))
              .map((chip) => chip.spans.map((sp) => sp.text).join(""))
              .join(" / ")
              .replace(/[█░▓▒]/gu, "")
              .replace(/\s+/gu, " "),
          })),
          monitors: cats.map((c) => ({
            ...categoryLink(c),
            state: hasAlarm(c) ? "警告あり" : s.live ? "警告なし" : "記録値",
            stale: !s.live,
          })),
        },
      ],
      "alert-panel",
    );
  },
  ({ s }) =>
    panel(
      "コスト・稼働",
      "COST & TIME",
      "costs",
      "04",
      [
        compareMetrics(
          s,
          ["sessionCost", "todayCost"],
          "Cost",
          "セッション費用 / 本日・全セッション",
        ),
        timeAllocation(s),
      ],
      "small-panel cost-panel",
    ),
  ({ s }) =>
    panel(
      "トークン・キャッシュ",
      "TOKEN FLOW",
      "tokens",
      "05",
      [
        tokenComposition(s),
        metrics([
          reading(s, "cacheHit", "good"),
          reading(s, "requests"),
          reading(s, "thinking"),
        ]),
      ],
      "small-panel token-panel",
    ),
  ({ s }) =>
    panel(
      "性能・集中度",
      "PERFORMANCE",
      "performance",
      "06",
      [
        rings(s, ["focus", "toolErrors"], "Focus & errors"),
        {
          ...compareMetrics(
            s,
            ["turnP50", "turnP90"],
            "Latency",
            "完了ターンの応答時間",
          ),
          layout: "range",
        },
      ],
      "small-panel performance-panel",
    ),
  ({ s }) =>
    panel(
      "作業・タスク",
      "WORK ACTIVITY",
      "work",
      "07",
      [activityColumns(s)],
      "small-panel work-panel",
    ),
  ({ s, cats }) =>
    panel(
      "Git・プルリクエスト",
      "CODE DELIVERY",
      "git",
      "08",
      [
        {
          kind: "text",
          style: "branch",
          text:
            cats
              .find((c) => c.name === "Git")
              ?.chips[0]?.spans.map((span) => span.text)
              .join("") ?? "ブランチ未取得",
        },
        gitBalance(s),
        metrics([reading(s, "changed"), reading(s, "staged")]),
      ],
      "small-panel git-panel",
    ),
  ({ s }) =>
    panel(
      "品質・開発習慣",
      "QUALITY SIGNALS",
      "quality",
      "09",
      [qualityTiles(s)],
      "small-panel quality-panel",
    ),
  ({ s }) =>
    panel(
      "エージェント・セッション",
      "AGENT ACTIVITY",
      "agents",
      "10",
      [
        {
          ...compareMetrics(
            s,
            ["explores", "edits", "prompts"],
            "Agent",
            "探索・編集・人間の入力",
          ),
          layout: "lollipop",
        },
        metrics([reading(s, "autonomy"), reading(s, "intervention")]),
      ],
      "small-panel agent-panel",
    ),
  ({ s }) =>
    panel(
      "端末・環境",
      "SYSTEM RESOURCES",
      "environment",
      "11",
      [
        compareMetrics(
          s,
          ["cpuLoad", "freeMemory", "freeDisk", "battery"],
          "Resources",
          "Load / 空き容量 / バッテリー",
          false,
        ),
      ],
      "small-panel system-readings",
    ),
];

// 既存の概観を保ち、研究指標の画面では単位と母数の揃う組だけを比較する。
const RESEARCH_PANELS: readonly PanelProjector[] = RESEARCH_PANEL_DEFINITIONS.map(
  (definition) =>
    ({ s }) =>
      panel(
        definition.title,
        definition.meta,
        definition.id,
        definition.number,
        [
          compareMetrics(
            s,
            definition.comparison.keys,
            definition.comparison.title,
            definition.comparison.note,
            definition.comparison.sharedScale,
          ),
          metrics(definition.readings.map((key) => reading(s, key))),
        ],
        "small-panel research-panel",
      ),
);

function detail(state: DashboardState, all: readonly Section[]): DetailVM {
  const route = parseRoute(state.hash),
    s = state.snapshot;
  const metricKey =
    route.kind === "metric" && Object.hasOwn(MEASUREMENTS, route.key)
      ? (route.key as MeasurementKey)
      : undefined;
  const definition = metricKey
    ? metricDetail(MEASUREMENTS[metricKey])
    : route.kind === "detail"
      ? DETAILS.get(route.key)
      : undefined;
  const category =
    route.kind === "category"
      ? all.flatMap((b) => b.categories).find((c) => c.name === route.key)
      : undefined;
  const selected = (name: string): boolean =>
    category
      ? name === category.name
      : !!definition &&
        (definition.categories.length === 0 ||
          definition.categories.includes(name));
  const query = state.query.trim().toLowerCase();
  const onlyAlerts = state.onlyAlerts || route.key === "alerts";
  const measured =
    s && !onlyAlerts
      ? (Object.keys(MEASUREMENTS) as MeasurementKey[])
          .filter((key) => {
            const { label, group, description: explanation } = MEASUREMENTS[key];
            return (
              (metricKey ? key === metricKey : selected(group)) &&
              `${label} ${group} ${measurementText(s, key)} ${explanation}`
                .toLowerCase()
                .includes(query)
            );
          })
          .map((key) => reading(s, key))
      : [];
  const bands = all.flatMap(({ band, categories }) => {
    const found = categories.filter(
      (c) =>
        selected(c.name) &&
        (!onlyAlerts || hasAlarm(c)) &&
        `${c.name} ${signalText([c], c.name) ?? ""}`
          .toLowerCase()
          .includes(query),
    );
    return found.length
      ? [
          {
            name: band.name,
            question: band.question,
            cards: found.map(
              (c): CardVM => ({
                id: anchor(band.band, c),
                title: c.name,
                alarm: hasAlarm(c),
                state: hasAlarm(c)
                  ? "● ALERT"
                  : s?.live
                    ? "● OBSERVED"
                    : "● RECORDED",
                chips: c.chips.map((chip) => ({
                  alarm: chip.spans.some((s) => s.alarm),
                  pieces: pieces(chip.spans),
                })),
              }),
            ),
          },
        ]
      : [];
  });
  const levels =
    s &&
    route.kind === "detail" &&
    ["limits", "context", "all"].includes(route.key)
      ? charts(s, state.rangeMinutes, true, state.hiddenSeries)
      : [];
  const diagrams =
    s && route.kind === "detail" && !onlyAlerts && !query
      ? [...PANELS, ...RESEARCH_PANELS]
          .map((project) =>
            project({ s, all, cats: all.flatMap((band) => band.categories) }),
          )
          .filter(
            (panel) =>
              panel.route !== "alerts" &&
              (route.key === "all" ||
                panel.route === route.key ||
                (route.key === "research" &&
                  [
                    "latency",
                    "tooling",
                    "diversity",
                    "budget",
                    "evidence",
                  ].includes(panel.route ?? ""))),
          )
          .map((panel) => ({ ...panel, route: undefined }))
      : [];
  return {
    title: definition?.title ?? category?.name ?? "表示先が見つかりません",
    description:
      definition?.description ??
      (category
        ? `${category.name} の取得済み指標をすべて表示します。`
        : "ダッシュボードから表示したい指標を選んでください。"),
    measurements: measured,
    bands,
    charts: [
      ...(route.key === "limits"
        ? levels.slice(0, 1)
        : route.key === "context"
          ? levels.slice(1)
          : levels),
      ...diagrams,
    ],
    emptyText:
      route.key === "alerts" && !query
        ? "取得した指標に警告はありません。"
        : "表示できる指標はありません。",
  };
}

function freshness(state: DashboardState): string {
  const s = state.snapshot;
  if (state.offline)
    return s
      ? `接続が途切れています。${dateTime(s.at)} に取得した値を表示中。`
      : "接続できません。更新ボタンから再接続できます。";
  if (!s) return "読み取り専用 · 5秒ごとに更新";
  if (!s.inputAt) return "初回描画待ち。取得できた環境情報を表示しています。";
  if (!s.live)
    return `記録値 · 最終描画 ${dateTime(s.inputAt)} · 古い入力から予測は行いません。`;
  return `最終描画 ${clock(s.inputAt, { seconds: true })} · 記録済みの実測値 · 読み取り専用`;
}

// データ選択・フィルタ・表示状態の判定は DOM を持たないこの境界で完結させる。
export function presentDashboard(state: DashboardState): DashboardVM {
  const s = state.snapshot,
    all = s ? sections(s.bands, s.groups) : [],
    cats = all.flatMap((b) => b.categories);
  const overview = parseRoute(state.hash).kind === "overview",
    details = detail(state, all);
  const fresh = freshness(state);
  const status = state.paused
    ? { kind: "paused", text: "更新を一時停止" }
    : state.offline
      ? { kind: "offline", text: "接続できません" }
      : !s
        ? { kind: "connecting", text: "接続中" }
        : s.live
          ? { kind: "live", text: `LIVE · ${clock(s.at, { seconds: true })}` }
          : { kind: "idle", text: "記録済みの値" };
  const panels = s
    ? [
        ...charts(s, state.rangeMinutes, false, state.hiddenSeries),
        ...PANELS.map((project) => project({ s, cats, all })),
      ]
    : [];
  const focused = overview
    ? panels.find((panel) => panel.route === state.focusedPanel)
    : undefined;
  return {
    revision: state.revision ?? 0,
    preview: focused?.route
      ? {
          route: focused.route,
          panel: { ...focused, route: undefined },
          detail: detail(
            {
              ...state,
              hash: `#detail/${focused.route}`,
              query: "",
              onlyAlerts: false,
            },
            all,
          ),
          freshness: fresh,
        }
      : undefined,
    hash: state.hash,
    overview,
    title: overview
      ? "Promari | Session Dashboard"
      : `${details.title} | Promari`,
    breadcrumb: overview ? "Overview" : details.title,
    status,
    freshness: fresh,
    freshnessClass: `freshness${state.offline ? " is-offline" : s && !s.live ? " is-stale" : ""}`,
    detailFreshness: `${fresh}${!state.offline && s?.limitsAt ? ` レート観測 ${dateTime(s.limitsAt)}。` : ""}`,
    updated: s
      ? `更新 ${clock(s.at, { seconds: true })} · ${state.paused ? "一時停止中" : "5秒ごと"}`
      : state.paused
        ? "一時停止中"
        : "更新待ち",
    version: s ? `psl ${s.version}` : "psl",
    controls: {
      busy: state.busy,
      paused: state.paused,
      pauseText: state.paused ? "▶ 再開" : "Ⅱ 一時停止",
      query: state.query,
      onlyAlerts: state.onlyAlerts,
      rangeMinutes: state.rangeMinutes,
      fullscreenLabel: state.fullscreen ? "全画面表示を終了" : "全画面表示",
      fullscreenTitle: state.fullscreenFailed
        ? "ブラウザの全画面表示をご利用ください"
        : state.fullscreen
          ? "全画面表示を終了"
          : "全画面表示",
    },
    summary: s ? summary(s, cats) : [],
    panels,
    categories: cats.map(categoryLink),
    detail: details,
  };
}
