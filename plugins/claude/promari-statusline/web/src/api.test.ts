import assert from "node:assert/strict";
import { test } from "node:test";
import { createSnapshotRepository } from "./api.ts";
import type { Snapshot } from "./model.ts";

const payload = {
  version: "test",
  at: "2026-10-06T01:00:00Z",
  inputAt: "2026-10-06T01:00:00Z",
  live: true,
  limitsAt: "2026-10-06T01:00:00Z",
  session: { name: "test", model: "test", project: "test", version: "test" },
  headline: {
    contextPct: 0,
    fiveHour: { usedPct: 0, resetsAt: "2026-10-06T06:00:00Z" },
    sevenDay: { usedPct: 0, resetsAt: "2026-10-13T01:00:00Z" },
    sessionUsd: 0,
    todayUsd: 0,
    running: 0,
  },
  bands: [],
  groups: [],
  rateHistory: [],
  codexHistory: [],
  contextHistory: [],
  measurements: {},
} satisfies Snapshot;

test("スナップショットをキャッシュせず読み込み、キャンセル信号を取得元に渡す", async () => {
  const controller = new AbortController();
  let received: AbortSignal | undefined;
  const repository = createSnapshotRepository(async (url, options) => {
    assert.equal(url, "/api/snapshot");
    assert.equal(options?.cache, "no-store");
    assert.equal(options?.method, undefined);
    received = options?.signal ?? undefined;
    return Response.json(payload);
  });
  assert.deepEqual(await repository.load(controller.signal), payload);
  assert.equal(received?.aborted, false);
  controller.abort();
  assert.equal(received?.aborted, true);
});

test("HTTP エラー・不正な JSON・通信失敗を ViewModel に伝える", async () => {
  const signal = new AbortController().signal;
  await assert.rejects(
    createSnapshotRepository(
      async () => new Response("停止中", { status: 503 }),
    ).load(signal),
    /503/u,
  );
  await assert.rejects(
    createSnapshotRepository(async () => new Response("{broken")).load(signal),
    SyntaxError,
  );
  const failure = new Error("接続切断");
  await assert.rejects(
    createSnapshotRepository(async () => {
      throw failure;
    }).load(signal),
    (error) => error === failure,
  );
});
