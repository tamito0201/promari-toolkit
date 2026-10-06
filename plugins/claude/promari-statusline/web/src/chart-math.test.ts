import assert from "node:assert/strict";
import { test } from "node:test";
import { observations, nearestObservation, cursorIndex } from "./chart-math.ts";

test("非表示・非数値を除き実測値だけを並べ、別時刻の値を補間しない", () => {
  const input = [
    {
      name: "Claude",
      color: "mint" as const,
      points: [
        { at: 20, value: 80 },
        { at: 10, value: 0 },
        { at: 30, value: NaN },
      ],
    },
    {
      name: "Codex",
      color: "codex" as const,
      points: [
        { at: 15, value: 25 },
        { at: 20, value: 50 },
      ],
    },
    {
      name: "非表示",
      color: "teal" as const,
      hidden: true,
      points: [{ at: 14, value: 90 }],
    },
  ];
  const before = structuredClone(input);
  const points = observations(input);
  assert.deepEqual(
    points.map((p) => [p.at, p.value]),
    [
      [10, 0],
      [15, 25],
      [20, 80],
      [20, 50],
    ],
  );
  assert.equal(nearestObservation(points, 14), 1);
  assert.equal(nearestObservation(points, -100), 0);
  assert.equal(nearestObservation(points, 100), 2);
  assert.equal(nearestObservation([], 0), -1);
  assert.equal(nearestObservation(points, NaN), -1);
  assert.deepEqual(input, before);
});
test("キーボードで先頭・末尾・同時刻の別系列を含め全実測点を選べる", () => {
  assert.equal(cursorIndex(1, "ArrowRight", 4), 2);
  assert.equal(cursorIndex(1, "ArrowLeft", 4), 0);
  assert.equal(cursorIndex(2, "Home", 4), 0);
  assert.equal(cursorIndex(0, "End", 4), 3);
  assert.equal(cursorIndex(0, "ArrowLeft", 4), 0);
  assert.equal(cursorIndex(3, "ArrowRight", 4), 3);
  assert.equal(cursorIndex(2, "Unknown", 4), 2);
  assert.equal(cursorIndex(0, "Home", 0), -1);
});
