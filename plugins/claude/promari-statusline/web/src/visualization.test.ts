import assert from "node:assert/strict";
import { test } from "node:test";
import type { Snapshot } from "./model.ts";
import { MEASUREMENTS, type MeasurementKey } from "./measurements.ts";
import {
  activityColumns,
  qualityTiles,
  timeAllocation,
  rateHeatmap,
  niceMaximum,
  axisNumber,
  instrument,
  percentageInstrument,
  compareMetrics,
  rings,
  tokenComposition,
  gitBalance,
} from "./visualization.ts";

function snapshot(values: Record<string, number | undefined> = {}): Snapshot {
  return {
    version: "test",
    at: "2026-10-06T01:00:00Z",
    live: true,
    headline: { running: 0 },
    session: {},
    bands: [],
    groups: [],
    rateHistory: [],
    codexHistory: [],
    contextHistory: [],
    measurements: Object.fromEntries(
      Object.entries(values).map(([key, value]) => [
        key,
        value === undefined ? { note: "観測待ち" } : { value },
      ]),
    ),
  };
}
test("全104指標に単位・目盛り・詳細リンクがあり、欠測とゼロを区別する", () => {
  const keys = Object.keys(MEASUREMENTS) as MeasurementKey[];
  assert.equal(keys.length, 104);
  for (const key of keys) {
    const missing = instrument(snapshot(), key);
    assert.equal(missing.value, undefined);
    assert.equal(missing.fraction, undefined);
    assert.equal(missing.tone, "muted");
    const zero = instrument(snapshot({ [key]: 0 }), key);
    assert.equal(zero.fraction, 0);
    assert.ok(Number.isFinite(zero.maximum) && zero.maximum > 0);
    assert.equal(zero.href, `#metric/${key}`);
    assert.ok(zero.maximumText && zero.basis);
  }
});
test("目盛りは有限の正数で、極小・巨大・端数を切り捨てない", () => {
  for (const value of [
    Number.MIN_VALUE,
    0.001,
    0.2,
    1,
    2.01,
    7,
    109,
    Number.MAX_VALUE,
  ]) {
    const maximum = niceMaximum(value);
    assert.ok(Number.isFinite(maximum));
    assert.ok(maximum >= value);
  }
  for (const value of [0, -1, NaN, Infinity])
    assert.equal(niceMaximum(value), 1);
  assert.equal(niceMaximum(32), 50);
  assert.equal(axisNumber(0), "0");
  assert.equal(axisNumber(1000), "1K");
  assert.equal(axisNumber(0.0002), "0.00020");
});
test("容量はGiBへ換算し、CPUの負荷とコア数は使用率に変換しない", () => {
  const s = snapshot({
    freeMemory: 2 ** 31,
    cpuLoad: 12,
    cores: 8,
    todoDone: 2,
    todoTotal: 5,
  });
  const memory = instrument(s, "freeMemory");
  assert.equal(memory.value, 2);
  assert.equal(memory.unit, "GiB");
  const load = instrument(s, "cpuLoad");
  assert.equal(load.percentage, false);
  assert.equal(load.value, 12);
  assert.equal(load.maximum, 20);
  assert.equal(load.reference?.fraction, 0.4);
  assert.match(load.basis, /参考線/u);
  assert.equal(instrument(s, "todoDone").reference?.fraction, 1);
});
test("同単位だけを共通目盛りで比較し、費用の期間を合算しない", () => {
  const s = snapshot({
    todayCost: 12,
    sessionCost: 3,
    freeMemory: 2 ** 30,
    battery: 0,
  });
  const costs = compareMetrics(
    s,
    ["todayCost", "sessionCost"],
    "Cost",
    "期間が異なります",
  );
  assert.deepEqual(
    costs.items.map((item) => item.maximum),
    [20, 20],
  );
  assert.deepEqual(
    costs.items.map((item) => item.fraction),
    [0.6, 0.15],
  );
  assert.equal(costs.total, undefined);
  assert.equal(costs.items[0]?.reference, undefined);
  const mixed = compareMetrics(s, ["todayCost", "freeMemory"], "System", "");
  assert.deepEqual(
    mixed.items.map((item) => item.maximum),
    [20, 1],
  );
  const separate = compareMetrics(
    s,
    ["todayCost", "sessionCost"],
    "Cost",
    "",
    false,
  );
  assert.deepEqual(
    separate.items.map((item) => item.maximum),
    [20, 5],
  );
  assert.equal(
    compareMetrics(s, ["battery"], "Battery", "").items[0]?.maximum,
    100,
  );
});
test("入出力の構成比はキャッシュを二重計上せず、欠測時に全体を推測しない", () => {
  const s = snapshot({
    inputTokens: 75,
    outputTokens: 25,
    cacheRead: 60,
    cacheWrite: 10,
  });
  const before = structuredClone(s);
  const flow = tokenComposition(s);
  assert.equal(flow.total, "100");
  assert.deepEqual(
    flow.items.map((item) => item.fraction),
    [0.75, 0.25],
  );
  assert.deepEqual(s, before);
  assert.equal(
    tokenComposition(snapshot({ inputTokens: 75 })).total,
    undefined,
  );
  assert.equal(
    tokenComposition(snapshot({ inputTokens: -1, outputTokens: 1 })).total,
    undefined,
  );
  const zero = tokenComposition(snapshot({ inputTokens: 0, outputTokens: 0 }));
  assert.equal(zero.total, "0");
  assert.ok(
    zero.items.every(
      (item) => item.fraction === undefined && item.basis === "入出力ともに0",
    ),
  );
});
test("指標ごとに警告の向きを区別し、割合の超過値も隠さない", () => {
  for (const [key, value, tone] of [
    ["battery", 19, "danger"],
    ["battery", 20, "good"],
    ["toolErrors", 1, "good"],
    ["toolErrors", 2, "caution"],
    ["toolErrors", 5, "danger"],
    ["rules", 1, "caution"],
    ["untested", 0, "good"],
    ["intervention", 20, "caution"],
  ] as const)
    assert.equal(instrument(snapshot({ [key]: value }), key).tone, tone);
  assert.equal(
    percentageInstrument("rate", "Rate", undefined).fraction,
    undefined,
  );
  assert.equal(percentageInstrument("rate", "Rate", Infinity).value, undefined);
  assert.equal(percentageInstrument("rate", "Rate", 89).tone, "accent");
  assert.equal(percentageInstrument("rate", "Rate", 90).tone, "danger");
  const over = percentageInstrument("rate", "Rate", 110, "#detail/limits");
  assert.equal(over.maximum, 200);
  assert.equal(over.value, 110);
  assert.equal(over.fraction, 0.55);
  assert.equal(
    rings(snapshot({ focus: 50 }), ["focus"], "Focus").items[0]?.fraction,
    0.5,
  );
  const diff = gitBalance(snapshot({ inserted: 5, deleted: 10 }));
  assert.equal(diff.layout, "balance");
  assert.deepEqual(
    diff.items.map((item) => item.fraction),
    [0.5, 1],
  );
});

test("作業の棒グラフと品質の分割ゲージは共通目盛りで比較する", () => {
  const s = snapshot({
    turns: 3,
    tools: 14,
    edited: 7,
    hooks: 2,
    tests: 5,
    builds: 3,
    rules: 0,
    untested: 2,
  });
  const work = activityColumns(s);
  assert.equal(work.layout, "columns");
  assert.deepEqual(
    work.items.map((item) => item.maximum),
    [20, 20, 20, 20],
  );
  const quality = qualityTiles(s);
  assert.equal(quality.layout, "waffle");
  assert.deepEqual(
    quality.items.map((item) => item.fraction),
    [1, 0.6, 0, 0.4],
  );
  assert.equal(quality.total, undefined);
});
test("時間配分は活動と休止だけを合計し、経過時間や深い作業を重複加算しない", () => {
  const allocation = timeAllocation(
    snapshot({ active: 90, idle: 30, deep: 80, wall: 200 }),
  );
  assert.equal(allocation.total, "120s");
  assert.deepEqual(
    allocation.items.map((item) => item.fraction),
    [0.75, 0.25],
  );
  assert.equal(timeAllocation(snapshot({ active: 10 })).total, undefined);
  assert.equal(
    timeAllocation(snapshot({ active: -1, idle: 2 })).total,
    undefined,
  );
  assert.equal(timeAllocation(snapshot({ active: 0, idle: 0 })).total, "0s");
  assert.ok(
    timeAllocation(snapshot({ active: 0, idle: 0 })).items.every(
      (item) => item.fraction === undefined,
    ),
  );
});
test("ヒートマップは時間区間の実測ピークを使い、境界・欠測・非表示を区別する", () => {
  const source = [
    {
      name: "Claude",
      points: [
        { at: 0, value: 0 },
        { at: 0.5, value: 20 },
        { at: 1, value: 60 },
        { at: 24, value: 90 },
        { at: 25, value: 99 },
        { at: -1, value: 99 },
        { at: 10, value: NaN },
      ],
    },
    { name: "Codex", hidden: true, points: [{ at: 3, value: 40 }] },
  ];
  const before = structuredClone(source);
  const heatmap = rateHeatmap(source, 0, 24);
  assert.equal(heatmap.rows.length, 1);
  const cells = heatmap.rows[0]!.cells;
  assert.equal(cells.length, 24);
  assert.equal(cells[0]?.peak, 20);
  assert.equal(cells[0]?.count, 2);
  assert.equal(cells[1]?.peak, 60);
  assert.equal(cells[23]?.peak, 90);
  assert.equal(cells[23]?.critical, true);
  assert.equal(cells[0]?.critical, false);
  assert.equal(cells[2]?.peak, undefined);
  assert.equal(cells[10]?.count, 0);
  assert.deepEqual(source, before);
  assert.equal(
    rateHeatmap([{ name: "Zero", points: [{ at: 0, value: 0 }] }], 0, 24)
      .rows[0]?.cells[0]?.peak,
    0,
  );
  assert.ok(
    rateHeatmap(source, 24, 24).rows[0]?.cells.every(
      (cell) => cell.peak === undefined,
    ),
  );
});
