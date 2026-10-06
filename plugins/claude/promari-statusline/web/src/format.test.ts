import assert from "node:assert/strict";
import { test } from "node:test";
import type { Group } from "./model.ts";
import {
  alarmed,
  anchor,
  barRatio,
  pieces,
  category,
  sections,
  signalValue,
  signalText,
} from "./categories.ts";
import { severity, duration as span, usd } from "./format.ts";
import { historyPoints, codexSeries } from "./series.ts";
import { parseRoute as detailRoute } from "./routes.ts";
import { measurementNumber, measurementText } from "./measurements.ts";

test("a titled group splits its title into emoji and name", () => {
  const g: Group = {
    band: 2,
    title: "🧠 Context",
    tone: "accent",
    chips: [{ spans: [{ text: "28%" }] }],
  };
  const c = category(g);
  assert.equal(c.emoji, "🧠");
  assert.equal(c.name, "Context");
  assert.equal(c.chips.length, 1);
});

test("Codex の実測時間枠を系列化し、primary を5時間枠と決めつけない", () => {
  const at = "2026-10-06T01:00:00Z";
  const next = "2026-10-06T01:01:00Z";
  const series = codexSeries([
    { at, primary: { usedPct: 0, windowMinutes: 10080 } },
    {
      at: next,
      primary: { usedPct: 20, windowMinutes: 10080 },
      secondary: { usedPct: 5, windowMinutes: 300 },
    },
    { at: next, secondary: { usedPct: 20, windowMinutes: 10080 } },
    { at, secondary: { usedPct: 1, windowMinutes: 30 } },
    { at: "invalid", primary: { usedPct: 99, windowMinutes: 300 } },
    { at, primary: { usedPct: 50, windowMinutes: 0 } },
    { at, primary: { usedPct: NaN, windowMinutes: 300 } },
    { at },
  ]);
  assert.deepEqual(series, [
    { name: "Codex · 30m", points: [{ at: Date.parse(at), value: 1 }] },
    { name: "Codex · 5h", points: [{ at: Date.parse(next), value: 5 }] },
    {
      name: "Codex · 7d",
      points: [
        { at: Date.parse(at), value: 0 },
        { at: Date.parse(next), value: 20 },
      ],
    },
  ]);
  assert.deepEqual(codexSeries([]), []);
});

test("a group without a title takes its header from the first span and drops the blank after it", () => {
  const g: Group = {
    band: 2,
    title: "",
    tone: "",
    chips: [
      {
        spans: [
          { text: "⚡ Claude", tone: "brand", bold: true },
          { text: " " },
          { text: "5h" },
        ],
      },
    ],
  };
  const c = category(g);
  assert.equal(c.name, "Claude");
  assert.equal(c.tone, "brand");
  assert.deepEqual(c.chips, [{ spans: [{ text: "5h" }] }]);
});

test("a header that is the whole chip leaves no empty chip", () => {
  const g: Group = {
    band: 2,
    title: "",
    tone: "",
    chips: [{ spans: [{ text: "🤖 Codex" }] }, { spans: [{ text: "7d" }] }],
  };
  assert.deepEqual(category(g).chips, [{ spans: [{ text: "7d" }] }]);
});

test("a block bar is read as a ratio; other text is not a bar", () => {
  assert.equal(barRatio("██░░░"), 0.4);
  assert.equal(barRatio("█████"), 1);
  assert.equal(barRatio("░░░░░"), 0);
  assert.equal(barRatio("28%"), null);
  assert.equal(barRatio(""), null);
});

test("every band is kept, empty or not, in the order given", () => {
  const bands = [
    { band: 1, name: "ALERTS", question: "a" },
    { band: 2, name: "LIMITS", question: "b" },
  ];
  const groups: Group[] = [
    { band: 2, title: "🧠 Context", tone: "", chips: [] },
  ];
  const s = sections(bands, groups);
  assert.deepEqual(
    s.map((x) => [x.band.name, x.categories.length]),
    [
      ["ALERTS", 0],
      ["LIMITS", 1],
    ],
  );
});

test("severity follows the status line's marks, inclusive at each mark", () => {
  assert.equal(severity(49.9), "good");
  assert.equal(severity(50), "caution");
  assert.equal(severity(80), "danger");
});

test("dollars and durations are written as the status line writes them", () => {
  assert.equal(usd(6.6281), "$6.63");
  assert.equal(usd(1052.4), "$1,052");
  assert.equal(span(59 * 60_000), "59m");
  assert.equal(span(4 * 3_600_000 + 4 * 60_000), "4h04m");
  assert.equal(span(2 * 86_400_000 + 20 * 3_600_000), "2d20h");
  assert.equal(span(-5_000), "0m");
});

test("alarmed finds the categories in alarm across bands, and anchors are stable ids", () => {
  const bands = [
    { band: 1, name: "ALERTS", question: "a" },
    { band: 2, name: "LIMITS", question: "b" },
  ];
  const groups: Group[] = [
    {
      band: 2,
      title: "🧠 Context",
      tone: "",
      chips: [{ spans: [{ text: "31%" }] }],
    },
    {
      band: 2,
      title: "",
      tone: "",
      chips: [
        {
          spans: [
            { text: "🤖 Codex" },
            { text: " " },
            { text: "100%", alarm: true },
          ],
        },
      ],
    },
  ];
  const found = alarmed(sections(bands, groups));
  assert.deepEqual(
    found.map((f) => [f.band, f.category.name]),
    [[2, "Codex"]],
  );
  const first = found[0];
  assert.ok(first);
  assert.equal(anchor(first.band, first.category), "cat-2-codex");
});

test("the filled and empty spans of a bar make one gauge, coloured like the filled part", () => {
  const got = pieces([
    { text: "5h", tone: "muted" },
    { text: " " },
    { text: "█", tone: "good" },
    { text: "░░░░", tone: "muted" },
    { text: " 19%", tone: "good" },
  ]);
  assert.deepEqual(got, [
    { kind: "text", span: { text: "5h", tone: "muted" } },
    { kind: "text", span: { text: " " } },
    { kind: "bar", ratio: 0.2, tone: "good" },
    { kind: "text", span: { text: " 19%", tone: "good" } },
  ]);
  assert.deepEqual(pieces([{ text: "░░░░░", tone: "muted" }]), [
    { kind: "bar", ratio: 0, tone: "muted" },
  ]);
});

test("履歴は欠測・未来・期間外を除外し、ゼロと元の観測時刻を保持する", () => {
  const points = [
    { at: 200, value: 80 },
    { at: 100, value: 0 },
    { at: 50, value: 30 },
    { at: 300, value: 90 },
    { at: 150, value: NaN },
    { at: NaN, value: 10 },
  ];
  assert.deepEqual(historyPoints(points, 100, 200), [
    { at: 100, value: 0 },
    { at: 200, value: 80 },
  ]);
  assert.equal(points[0]?.at, 200);
  assert.deepEqual(historyPoints(points, 400, 500), []);
});

test("主要指標は分類を区別し、未取得とゼロを混同しない", () => {
  const all = sections(
    [{ band: 5, name: "METRICS", question: "" }],
    [
      {
        band: 5,
        title: "📦 Cache",
        tone: "note",
        chips: [{ spans: [{ text: "Hit " }, { text: "0% TTL 1h" }] }],
      },
      {
        band: 5,
        title: "📊 Tokens",
        tone: "info",
        chips: [{ spans: [{ text: "Σ In 4.81M / Out 126k" }] }],
      },
    ],
  ).flatMap((s) => s.categories);
  assert.equal(signalValue(all, "Cache", /Hit ([\d.]+%)/u), "0%");
  assert.equal(signalValue(all, "Tokens", /Σ In (\S+)/u), "4.81M");
  assert.equal(signalValue(all, "Tokens", /\/ Out (\S+)/u), "126k");
  assert.equal(signalValue(all, "Cache", /Save (\S+)/u), undefined);
  assert.equal(signalValue(all, "Missing", /Hit (\S+)/u), undefined);
  assert.equal(signalText(all, "Cache"), "Hit 0% TTL 1h");
  assert.equal(signalText(all, "Missing"), undefined);
});

test("詳細画面のリンクは再読み込みでき、不正な URL でも例外を投げない", () => {
  assert.deepEqual(detailRoute("#overview"), { kind: "overview", key: "" });
  assert.deepEqual(detailRoute(""), { kind: "overview", key: "" });
  assert.deepEqual(detailRoute("#main"), { kind: "overview", key: "" });
  assert.deepEqual(detailRoute("#detail/tokens"), {
    kind: "detail",
    key: "tokens",
  });
  assert.deepEqual(detailRoute("#category/My%20Category"), {
    kind: "category",
    key: "My Category",
  });
  assert.deepEqual(detailRoute("#category/%E0%A4"), {
    kind: "missing",
    key: "",
  });
  assert.deepEqual(detailRoute("#detail/a/b"), { kind: "missing", key: "" });
});

test("集計値は表示チップがなくてもゼロを保持し、算出不能には理由を表示する", () => {
  const s = {
    measurements: {
      changed: { value: 0 },
      blockCost: { note: "稼働枠なし" },
      costTurn: { value: 0.0035 },
      focus: { value: NaN },
      parallel: { value: 0.00036 },
      active: { value: 6.5 },
      idle: { value: 90 },
      freeMemory: { value: 2 ** 30 },
      cacheHit: { value: 0 },
      commitStreak: { value: 14 },
      cpuLoad: { value: 1.25 },
    },
  };
  assert.equal(measurementNumber(s, "changed"), 0);
  assert.equal(measurementText(s, "changed"), "0");
  assert.equal(measurementNumber(s, "focus"), undefined);
  assert.equal(measurementText(s, "focus"), "1分観測待ち");
  assert.equal(measurementText(s, "blockCost"), "稼働枠なし");
  assert.equal(measurementText(s, "turnP50"), "完了記録待ち");
  assert.equal(measurementText(s, "costLine"), "追加行待ち");
  assert.equal(measurementText(s, "costTurn"), "$0.0035");
  assert.equal(measurementText(s, "parallel"), "×0.00036");
  assert.equal(measurementText(s, "active"), "6.5s");
  assert.equal(measurementText(s, "idle"), "1m");
  assert.equal(measurementText(s, "freeMemory"), "1 GiB");
  assert.equal(measurementText(s, "cacheHit"), "0%");
  assert.equal(measurementText(s, "commitStreak"), "14d");
  assert.equal(measurementText(s, "cpuLoad"), "1.3");
  assert.equal(measurementText(s, "requests"), "観測待ち");
});
