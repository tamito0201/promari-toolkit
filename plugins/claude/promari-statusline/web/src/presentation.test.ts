import assert from "node:assert/strict";
import { test } from "node:test";
import type { Group, Snapshot } from "./model.ts";
import { MEASUREMENTS } from "./measurements.ts";
import { RESEARCH_MEASUREMENTS } from "./research.ts";
import {
  presentDashboard,
  type DashboardState,
  type ChartVM,
} from "./presentation.ts";

const AT = "2026-10-06T01:00:00Z";
function group(name: string, text: string, alarm = false): Group {
  return {
    band: 1,
    title: `◈ ${name}`,
    tone: "",
    chips: [{ spans: [{ text, alarm }] }],
  };
}
function snapshot(change: Partial<Snapshot> = {}): Snapshot {
  return {
    version: "test",
    at: AT,
    inputAt: AT,
    limitsAt: AT,
    live: true,
    headline: { running: 0 },
    session: {},
    measurements: {},
    bands: [{ band: 1, name: "測定値", question: "現在の状態" }],
    groups: [],
    rateHistory: [],
    contextHistory: [],
    codexHistory: [],
    ...change,
  };
}
function state(change: Partial<DashboardState> = {}): DashboardState {
  return {
    snapshot: snapshot(),
    hash: "#overview",
    rangeMinutes: 60,
    query: "",
    onlyAlerts: false,
    paused: false,
    busy: false,
    offline: false,
    visible: true,
    fullscreen: false,
    fullscreenFailed: false,
    ...change,
  };
}
function chart(
  view: ReturnType<typeof presentDashboard>,
  index: number,
): ChartVM {
  const block = view.panels[index]?.blocks[0];
  assert.equal(block?.kind, "chart");
  assert.ok(block && block.kind === "chart");
  return block;
}

test("8つの主要指数・11パネル・104集計値を生成し、ゼロも値として表示する", () => {
  const s = snapshot({
    headline: {
      running: 0,
      contextPct: 0,
      fiveHour: { usedPct: 0, resetsAt: AT },
      sevenDay: { usedPct: 0 },
      todayUsd: 0,
      sessionUsd: 0,
    },
    measurements: Object.fromEntries(
      Object.keys(MEASUREMENTS).map((key) => [key, { value: 0 }]),
    ),
    groups: [
      group("Codex", "7d ░░░░░ 0%"),
      group("Git", "feature/example"),
      group("Due", "期限なし"),
      group("PR", "CI 成功"),
      group("Env", "Mode plan"),
      group("Sessions", "0 running"),
      group("Meta", "psl test"),
    ],
    session: { model: "テストモデル" },
  });
  const before = structuredClone(s);
  const view = presentDashboard(state({ snapshot: s, hash: "#detail/all" }));
  assert.equal(view.summary.length, 8);
  assert.equal(view.panels.length, 11);
  assert.equal(view.detail.measurements.length, 104);
  assert.equal(
    view.detail.measurements.every((metric) => metric.known),
    true,
  );
  assert.equal(
    view.summary.find((stat) => stat.label === "Codex · 7d")?.value,
    "0%",
  );
  assert.equal(
    view.summary.find((stat) => stat.label === "Today cost")?.value,
    "$0.00",
  );
  assert.equal(
    view.summary.find((stat) => stat.label === "Burn rate")?.value,
    "$0.00",
  );
  assert.equal(view.summary.find((stat) => stat.label === "Context")?.ratio, 0);
  assert.equal(
    view.detail.measurements.find((metric) => metric.key === "staged")?.value,
    "0",
  );
  assert.equal(view.detail.charts.length, 15);
  assert.deepEqual(s, before);
});

test("研究画面は49指標と5図表に観測数・算出根拠・一次情報を添える", () => {
  const s = snapshot({
    measurements: {
      toolErrorRate: { value: 0, note: "主会話の対応済み結果 · n=10" },
      turnP99: { note: "記録 20 / 必要 100 件" },
    },
  });
  const view = presentDashboard(
    state({ snapshot: s, hash: "#detail/research" }),
  );
  assert.equal(view.detail.measurements.length, 49);
  assert.equal(view.detail.charts.length, 5);
  assert.ok(
    view.detail.measurements.every((m) =>
      m.source?.href.startsWith("https://"),
    ),
  );
  const error = view.detail.measurements.find((m) => m.key === "toolErrorRate");
  assert.equal(error?.known, true);
  assert.equal(error?.value, "0%");
  assert.match(error?.explanation ?? "", /n=10/u);
  assert.equal(error?.visual?.tone, "info");
  const missing = view.detail.measurements.find((m) => m.key === "turnP99");
  assert.equal(missing?.known, false);
  assert.match(missing?.value ?? "", /必要 100 件/u);
  assert.equal(missing?.visual?.tone, "muted");
  for (const [route, count] of [
    ["latency", 9],
    ["tooling", 17],
    ["diversity", 6],
    ["budget", 10],
    ["evidence", 7],
  ] as const) {
    const detail = presentDashboard(
      state({ snapshot: s, hash: `#detail/${route}` }),
    ).detail;
    assert.equal(detail.measurements.length, count);
    assert.equal(detail.charts.length, 1);
  }
});

test("未取得をゼロに変換せず、算出理由とセッション平均の代替値を表示する", () => {
  const view = presentDashboard(
    state({
      snapshot: snapshot({
        measurements: {
          blockCost: { note: "稼働枠なし" },
          sessionBurn: { value: 0.5 },
          todoDone: { value: 1 },
        },
      }),
      hash: "#detail/all",
    }),
  );
  assert.equal(view.summary[0]?.value, "—");
  assert.equal(
    view.summary.find((stat) => stat.label === "Codex")?.ratio,
    undefined,
  );
  assert.equal(
    view.summary.find((stat) => stat.label === "Session burn")?.value,
    "$0.50",
  );
  const missing = view.detail.measurements.find(
    (metric) => metric.key === "blockCost",
  );
  assert.equal(missing?.known, false);
  assert.equal(missing?.value, "稼働枠なし");
  const work = view.panels.find((panel) => panel.route === "work")?.blocks[0];
  assert.ok(work?.kind === "visual");
  assert.equal(
    work.items.every((item) => item.value === undefined),
    true,
  );
  assert.equal(missing?.visual?.fraction, undefined);
});

test("推移は Claude と Codex の実測時間枠を保持し、期間外や未来の点を作らない", () => {
  const s = snapshot({
    rateHistory: [
      { at: "2026-10-06T00:50:00Z", fiveHour: 0, sevenDay: 20 },
      { at: "2026-10-06T00:00:00Z", fiveHour: 10 },
      { at: "2026-10-06T01:01:00Z", fiveHour: 90 },
      { at: "invalid", fiveHour: 80 },
    ],
    codexHistory: [
      {
        at: "2026-10-06T00:50:00Z",
        primary: { windowMinutes: 10080, usedPct: 30 },
        secondary: { windowMinutes: 300, usedPct: 0 },
      },
    ],
    contextHistory: [{ at: "2026-10-06T00:52:00Z", tokens: 0 }],
  });
  const before = structuredClone(s);
  const view = presentDashboard(
    state({ snapshot: s, rangeMinutes: 15, hash: "#detail/limits" }),
  );
  const rate = chart(view, 0);
  assert.deepEqual(
    rate.series.map((s) => s.name),
    ["Claude · 5h", "Claude · 7d", "Codex · 5h", "Codex · 7d"],
  );
  assert.deepEqual(
    rate.series.map((s) => s.points.map((p) => p.value)),
    [[0], [20], [0], [30]],
  );
  assert.equal(
    rate.series.every(
      (s) => s.points[0]?.at === Date.parse("2026-10-06T00:50:00Z"),
    ),
    true,
  );
  assert.equal(view.detail.charts.length, 1);
  assert.equal(view.detail.charts[0]?.route, undefined);
  assert.deepEqual(chart(view, 1).series[0]?.points, [
    { at: Date.parse("2026-10-06T00:52:00Z"), value: 0 },
  ]);
  const context = presentDashboard(
    state({ snapshot: s, hash: "#detail/context", rangeMinutes: 180 }),
  );
  assert.equal(context.detail.charts[0]?.title, "コンテキストの推移");
  assert.deepEqual(s, before);
});

test("カテゴリ・検索・警告フィルタを組み合わせ、算出根拠も検索する", () => {
  const s = snapshot({
    groups: [
      group("Cache", "Hit 0% TTL 1h"),
      group("Tokens", "Input 0"),
      group("Codex", "7d ███░░ 95%", true),
    ],
  });
  let view = presentDashboard(
    state({ snapshot: s, hash: "#detail/tokens", query: "  ttl  " }),
  );
  assert.deepEqual(
    view.detail.bands.flatMap((band) => band.cards.map((card) => card.title)),
    ["Cache"],
  );
  assert.equal(view.detail.measurements.length, 0);
  view = presentDashboard(state({ snapshot: s, hash: "#category/Cache" }));
  assert.equal(view.detail.title, "Cache");
  assert.equal(view.detail.measurements.length, 3);
  view = presentDashboard(
    state({ snapshot: s, hash: "#detail/all", query: "重複除外" }),
  );
  assert.deepEqual(
    view.detail.measurements.map((metric) => metric.key),
    ["inputTokens", "requests"],
  );
  view = presentDashboard(
    state({ snapshot: s, hash: "#detail/all", onlyAlerts: true }),
  );
  assert.equal(view.detail.measurements.length, 0);
  assert.deepEqual(
    view.detail.bands.flatMap((band) => band.cards.map((card) => card.title)),
    ["Codex"],
  );
  assert.equal(view.detail.bands[0]?.cards[0]?.state, "● ALERT");
  assert.equal(
    view.categories.find((category) => category.label === "Codex")?.alarm,
    true,
  );
  const alerts = view.panels.find((panel) => panel.route === "alerts")
    ?.blocks[0];
  assert.ok(alerts?.kind === "alerts");
  assert.equal(alerts.count, 1);
  assert.equal(alerts.items[0]?.text, "7d 95%");
  view = presentDashboard(state({ snapshot: s, hash: "#detail/alerts" }));
  assert.equal(view.detail.bands[0]?.cards[0]?.title, "Codex");
});

test("古い入力・初回待ち・接続断を区別し、観測時刻を表示する", () => {
  const s = snapshot({
    live: false,
    groups: [group("Cache", "Hit 0%"), group("Session", "記録済み")],
  });
  const view = presentDashboard(state({ snapshot: s, hash: "#detail/all" }));
  assert.equal(view.status.kind, "idle");
  assert.match(view.freshness, /古い入力から予測は行いません/u);
  assert.match(view.freshnessClass, /is-stale/u);
  assert.match(view.detailFreshness, /レート記録/u);
  assert.equal(view.detail.bands[0]?.cards[0]?.state, "● RECORDED");
  const { inputAt: _inputAt, ...withoutInput } = s;
  assert.match(
    presentDashboard(state({ snapshot: withoutInput })).freshness,
    /初回描画待ち/u,
  );
  assert.match(
    presentDashboard(state({ snapshot: undefined })).freshness,
    /読み取り専用/u,
  );
  assert.equal(
    presentDashboard(state({ snapshot: undefined })).summary.length,
    0,
  );
  assert.equal(
    presentDashboard(state({ snapshot: undefined, paused: true })).updated,
    "一時停止中",
  );
  assert.match(
    presentDashboard(state({ snapshot: undefined, offline: true })).freshness,
    /接続できません/u,
  );
  assert.doesNotMatch(
    presentDashboard(state({ offline: true })).detailFreshness,
    /レート記録/u,
  );
});

test("不明なリンクと警告なしの空状態でも例外を投げず案内する", () => {
  for (const hash of [
    "#detail/unknown",
    "#category/unknown",
    "#category/%E0%A4",
  ]) {
    const view = presentDashboard(state({ hash }));
    assert.equal(view.overview, false);
    assert.equal(view.detail.title, "表示先が見つかりません");
    assert.equal(view.detail.measurements.length + view.detail.bands.length, 0);
  }
  assert.match(
    presentDashboard(state({ hash: "#detail/alerts" })).detail.emptyText,
    /警告はありません/u,
  );
  assert.match(
    presentDashboard(state({ hash: "#detail/alerts", query: "一致しない" }))
      .detail.emptyText,
    /表示できる指標はありません/u,
  );
});

test("全指標の直リンクは対象の可視化と算出根拠へ移動し、不明な指標を偽装しない", () => {
  for (const key of Object.keys(MEASUREMENTS)) {
    const view = presentDashboard(state({ hash: `#metric/${key}` }));
    assert.equal(view.overview, false);
    assert.deepEqual(
      view.detail.measurements.map((item) => item.key),
      [key],
    );
    assert.equal(view.detail.measurements[0]?.visual?.href, `#metric/${key}`);
  }
  assert.equal(
    presentDashboard(state({ hash: "#metric/unknown" })).detail.title,
    "表示先が見つかりません",
  );
  const view = presentDashboard(
    state({ hiddenSeries: ["Claude · 5h"], hash: "#detail/limits" }),
  );
  assert.equal(chart(view, 0).series[0]?.hidden, true);
  const detailChart = view.detail.charts[0]?.blocks[0];
  assert.ok(detailChart?.kind === "chart");
  assert.equal(detailChart.series[0]?.hidden, true);
});

test("詳細ページは分類に合う比較図を拡大し、検索中や警告絞り込み中に無関係な図を表示しない", () => {
  for (const key of [
    "costs",
    "tokens",
    "performance",
    "work",
    "git",
    "quality",
    "agents",
    "environment",
  ]) {
    const detail = presentDashboard(state({ hash: `#detail/${key}` })).detail;
    assert.equal(detail.charts.length, 1);
    assert.equal(detail.charts[0]?.route, undefined);
  }
  assert.equal(
    presentDashboard(state({ hash: "#detail/quality", query: "tests" })).detail
      .charts.length,
    0,
  );
  assert.equal(
    presentDashboard(state({ hash: "#detail/quality", onlyAlerts: true }))
      .detail.charts.length,
    0,
  );
  const limits = chart(
    presentDashboard(state({ hiddenSeries: ["Claude · 5h"] })),
    0,
  );
  assert.equal(
    limits.heatmap?.rows.some((row) => row.label === "Claude · 5h"),
    false,
  );
});

test("拡大表示は選択した図と全数値・算出理由を投影し、画面遷移や不明な対象では開かない", () => {
  const view = presentDashboard(state({ focusedPanel: "performance" }));
  assert.equal(view.preview?.route, "performance");
  assert.equal(view.preview?.panel.route, undefined);
  assert.ok(
    view.preview?.detail.measurements.some(
      (item) => item.key === "turnP50" && item.value === "完了記録待ち",
    ),
  );
  assert.ok(
    view.preview?.detail.measurements.some(
      (item) => item.key === "focus" && item.value === "1分稼働待ち",
    ),
  );
  assert.equal(
    presentDashboard(state({ focusedPanel: "unknown" })).preview,
    undefined,
  );
  assert.equal(
    presentDashboard(
      state({ focusedPanel: "performance", hash: "#detail/performance" }),
    ).preview,
    undefined,
  );
});

test("研究の49指標は同じ主題の観測パネルが吸収し、1枚目に全部出す", () => {
  const s = snapshot();
  const view = presentDashboard(state({ snapshot: s }));
  // 研究の専用パネルを足さず、既存の11パネルの図と数値だけで全指標を被覆する。
  assert.equal(view.panels.length, 11);
  const listed = new Set<string>();
  for (const panel of view.panels) {
    for (const block of panel.blocks) {
      if (block.kind === "metrics")
        for (const item of block.items) if (item.key) listed.add(item.key);
      if (block.kind === "visual")
        for (const item of block.items) listed.add(item.key);
    }
  }
  for (const key of Object.keys(RESEARCH_MEASUREMENTS))
    assert.ok(listed.has(key), `概観に出ない研究指標: ${key}`);
});
