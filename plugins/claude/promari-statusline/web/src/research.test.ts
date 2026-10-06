import assert from "node:assert/strict";
import { test } from "node:test";
import { readFile } from "node:fs/promises";
import { RESEARCH_MEASUREMENTS } from "./research.ts";
import {
  MEASUREMENTS,
  measurementSource as researchSource,
} from "./measurements.ts";

test("49指標の分類・単位・定義と7つの一次情報を欠かさない", () => {
  assert.equal(Object.keys(RESEARCH_MEASUREMENTS).length, 49);
  const sources = new Set<string>();
  for (const [key, definition] of Object.entries(RESEARCH_MEASUREMENTS)) {
    assert.deepEqual(
      MEASUREMENTS[key as keyof typeof MEASUREMENTS],
      definition,
    );
    assert.ok(Object.values(definition).every(Boolean));
    const source = researchSource(key);
    assert.ok(source?.label);
    const url = new URL(source.href);
    assert.equal(url.protocol, "https:");
    sources.add(source.href);
  }
  assert.equal(sources.size, 7);
  assert.equal(researchSource("unknown"), undefined);
  assert.equal(researchSource("toString"), undefined);
  assert.equal(researchSource("__proto__"), undefined);
});

test("研究指標のキー・分類・ラベル・単位が Go と共有する契約に一致する", async () => {
  const contract: unknown = JSON.parse(
    await readFile(
      new URL("../../contracts/research-measurements.json", import.meta.url),
      "utf8",
    ),
  );
  assert.deepEqual(
    Object.entries(RESEARCH_MEASUREMENTS).map(([key, definition]) => ({
      key,
      category: definition.group,
      label: definition.label,
      unit: definition.unit,
    })),
    contract,
  );
});
