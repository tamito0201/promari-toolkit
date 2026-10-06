import assert from "node:assert/strict";
import { test } from "node:test";
import { setImmediate } from "node:timers/promises";
import type { Snapshot } from "./model.ts";
import type { DashboardVM } from "./presentation.ts";
import { createDashboardViewModel } from "./view-model.ts";

function observable<T>(initial: T) {
  let value = initial;
  const listeners = new Set<(value: T) => void>();
  return {
    current: () => value,
    subscribe(listener: (value: T) => void) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    set(next: T) {
      value = next;
      for (const listener of listeners) listener(value);
    },
    get subscribers() {
      return listeners.size;
    },
  };
}
function snapshot(version = "test"): Snapshot {
  return {
    version,
    at: "2026-10-06T01:00:00Z",
    live: true,
    headline: { running: 1 },
    session: {},
    measurements: {},
    bands: [],
    groups: [],
    rateHistory: [],
    contextHistory: [],
    codexHistory: [],
  };
}
function harness(visible = true, hash = "#overview") {
  const navigation = observable(hash),
    visibility = observable(visible),
    fullscreen = observable(false);
  const requests: {
    signal: AbortSignal;
    resolve(value: Snapshot): void;
    reject(reason: Error): void;
  }[] = [];
  const timers = new Set<{ delay: number; run(): void }>();
  let fullscreenCalls = 0,
    fullscreenFails = false;
  const model = createDashboardViewModel({
    repository: {
      load(signal) {
        return new Promise<Snapshot>((resolve, reject) => {
          requests.push({ signal, resolve, reject });
        });
      },
    },
    scheduler: {
      after(delay, run) {
        const timer = { delay, run };
        timers.add(timer);
        return () => {
          timers.delete(timer);
        };
      },
    },
    navigation: { ...navigation, go: navigation.set },
    visibility,
    fullscreen: {
      ...fullscreen,
      async toggle() {
        fullscreenCalls++;
        if (fullscreenFails) throw new Error("全画面表示不可");
        fullscreen.set(!fullscreen.current());
      },
    },
  });
  const observed: DashboardVM[] = [];
  const unsubscribe = model.subscribe((state) => observed.push(state));
  return {
    model,
    requests,
    timers,
    navigation,
    visibility,
    fullscreen,
    observed,
    unsubscribe,
    get state() {
      const state = observed.at(-1);
      assert.ok(state);
      return state;
    },
    get fullscreenCalls() {
      return fullscreenCalls;
    },
    failFullscreen() {
      fullscreenFails = true;
    },
    request(index: number) {
      const request = requests[index];
      assert.ok(request);
      return request;
    },
    tick() {
      const timer = [...timers][0];
      assert.ok(timer);
      timers.delete(timer);
      timer.run();
    },
  };
}

test("取得の完了から5秒後に更新し、実行中の手動更新を重複させない", async () => {
  const h = harness();
  assert.equal(h.state.status.kind, "connecting");
  await h.model.actions.refresh();
  assert.equal(h.requests.length, 0);
  h.model.start();
  h.model.start();
  assert.equal(h.requests.length, 1);
  assert.equal(h.state.controls.busy, true);
  assert.equal(h.timers.size, 0);
  await h.model.actions.refresh();
  assert.equal(h.requests.length, 1);
  h.request(0).resolve(snapshot());
  await setImmediate();
  assert.equal(h.state.status.kind, "live");
  assert.equal(h.state.controls.busy, false);
  assert.equal([...h.timers][0]?.delay, 5_000);
  h.tick();
  assert.equal(h.requests.length, 2);
  assert.equal(h.timers.size, 0);
  h.model.dispose();
});

test("一時停止で通信を中止し、再開後に届いた古い応答で上書きしない", async () => {
  const h = harness();
  h.model.start();
  h.model.actions.togglePause();
  assert.equal(h.request(0).signal.aborted, true);
  assert.equal(h.state.status.kind, "paused");
  assert.equal(h.state.controls.busy, false);
  h.model.actions.togglePause();
  assert.equal(h.requests.length, 2);
  h.request(1).resolve(snapshot("new"));
  await setImmediate();
  h.request(0).resolve(snapshot("old"));
  await setImmediate();
  assert.equal(h.state.version, "psl new");
  assert.equal(h.state.controls.busy, false);
  assert.equal(h.timers.size, 1);
  h.model.dispose();
});

test("一時停止中も手動更新でき、自動更新は再開しない", async () => {
  const h = harness();
  h.model.actions.togglePause();
  h.model.start();
  assert.equal(h.requests.length, 0);
  const refresh = h.model.actions.refresh();
  h.request(0).resolve(snapshot());
  await refresh;
  assert.equal(h.state.controls.paused, true);
  assert.equal(h.state.status.kind, "paused");
  assert.equal(h.state.summary.length, 8);
  assert.equal(h.timers.size, 0);
  assert.match(h.state.updated, /一時停止中/u);
  h.model.dispose();
});

test("非表示では更新せず、再表示時に更新する。一時停止は維持する", async () => {
  const h = harness(false);
  h.model.start();
  assert.equal(h.requests.length, 0);
  h.visibility.set(true);
  assert.equal(h.requests.length, 1);
  h.visibility.set(false);
  assert.equal(h.request(0).signal.aborted, true);
  h.request(0).reject(new Error("中止済み"));
  await setImmediate();
  assert.notEqual(h.state.status.kind, "offline");
  h.visibility.set(true);
  h.request(1).resolve(snapshot());
  await setImmediate();
  assert.equal(h.timers.size, 1);
  h.visibility.set(false);
  assert.equal(h.timers.size, 0);
  h.model.actions.togglePause();
  h.visibility.set(true);
  assert.equal(h.requests.length, 2);
  h.model.actions.togglePause();
  assert.equal(h.requests.length, 3);
  h.model.dispose();
});

test("通信失敗時に最後の取得値を保持し、次回成功で復帰する", async () => {
  const h = harness();
  h.model.start();
  h.request(0).reject(new Error("接続失敗"));
  await setImmediate();
  assert.equal(h.state.status.kind, "offline");
  assert.match(h.state.freshness, /接続できません/u);
  h.tick();
  h.request(1).resolve(snapshot("last"));
  await setImmediate();
  h.tick();
  h.request(2).reject(new Error("接続切断"));
  await setImmediate();
  assert.equal(h.state.status.kind, "offline");
  assert.equal(h.state.version, "psl last");
  assert.match(h.state.freshness, /取得した値を表示中/u);
  assert.match(h.state.freshnessClass, /is-offline/u);
  h.model.actions.togglePause();
  assert.equal(h.state.status.kind, "paused");
  h.model.actions.togglePause();
  h.request(3).resolve(snapshot("recovered"));
  await setImmediate();
  assert.equal(h.state.status.kind, "live");
  assert.equal(h.state.version, "psl recovered");
  h.model.dispose();
});

test("詳細への直リンクと履歴移動で表示を更新し、検索と警告フィルタを解除する", () => {
  const h = harness(false, "#detail/tokens");
  h.model.start();
  assert.equal(h.state.detail.title, "トークンとキャッシュ");
  h.model.actions.search("Cache");
  h.model.actions.toggleAlerts();
  h.navigation.set("#detail/tokens");
  assert.equal(h.state.controls.query, "Cache");
  assert.equal(h.state.controls.onlyAlerts, true);
  h.model.actions.setRange(15);
  assert.equal(h.state.controls.rangeMinutes, 15);
  h.model.actions.setRange(0);
  h.model.actions.setRange(NaN);
  assert.equal(h.state.controls.rangeMinutes, 15);
  h.model.actions.setRange(180);
  assert.equal(h.state.controls.rangeMinutes, 180);
  h.model.actions.navigate("#detail/limits");
  assert.equal(h.state.detail.title, "レート制限");
  assert.equal(h.state.controls.query, "");
  assert.equal(h.state.controls.onlyAlerts, false);
  h.navigation.set("#overview");
  assert.equal(h.state.overview, true);
  assert.equal(h.requests.length, 0);
  h.model.dispose();
});

test("全画面表示の失敗を通信失敗と混同せず、ブラウザ側の解除も反映する", async () => {
  const h = harness(false);
  h.model.start();
  const first = h.model.actions.toggleFullscreen();
  const duplicate = h.model.actions.toggleFullscreen();
  await Promise.all([first, duplicate]);
  assert.equal(h.fullscreenCalls, 1);
  assert.equal(h.state.controls.fullscreenLabel, "全画面表示を終了");
  h.fullscreen.set(false);
  assert.equal(h.state.controls.fullscreenLabel, "全画面表示");
  h.failFullscreen();
  await h.model.actions.toggleFullscreen();
  assert.match(h.state.controls.fullscreenTitle, /ブラウザ/u);
  assert.equal(h.state.status.kind, "connecting");
  h.model.dispose();
});

test("購読解除と破棄で通知・通信・タイマー・環境イベントを後片付けする", async () => {
  const h = harness();
  h.model.start();
  assert.equal(h.navigation.subscribers, 1);
  h.unsubscribe();
  const notifications = h.observed.length;
  h.model.actions.search("通知しない");
  assert.equal(h.observed.length, notifications);
  h.model.dispose();
  h.model.dispose();
  h.model.start();
  assert.equal(h.request(0).signal.aborted, true);
  assert.equal(h.timers.size, 0);
  assert.equal(
    h.navigation.subscribers +
      h.visibility.subscribers +
      h.fullscreen.subscribers,
    0,
  );
  h.model.subscribe(() => assert.fail("破棄後に通知された"))();
  h.request(0).resolve(snapshot("late"));
  await setImmediate();
  await h.model.actions.refresh();
  h.model.actions.togglePause();
  h.model.actions.setRange(15);
  h.model.actions.search("破棄後");
  h.model.actions.toggleAlerts();
  h.model.actions.navigate("#detail/all");
  await h.model.actions.toggleFullscreen();
  assert.equal(h.requests.length, 1);
  assert.equal(h.fullscreenCalls, 0);
  assert.equal(h.navigation.current(), "#overview");
});

test("系列の選択は期間・詳細遷移・更新後も維持し、破棄後は変更しない", async () => {
  const h = harness();
  h.model.start();
  h.request(0).resolve(snapshot());
  await setImmediate();
  function hidden(): boolean | undefined {
    const block = h.state.panels[0]?.blocks[0];
    assert.ok(block?.kind === "chart");
    return block.series[0]?.hidden;
  }
  h.model.actions.toggleSeries("Claude · 5h");
  assert.equal(hidden(), true);
  h.model.actions.setRange(180);
  h.model.actions.navigate("#detail/limits");
  h.tick();
  h.request(1).resolve(snapshot("updated"));
  await setImmediate();
  assert.equal(hidden(), true);
  h.model.actions.toggleSeries("Claude · 5h");
  assert.equal(hidden(), false);
  h.model.dispose();
  h.model.actions.toggleSeries("Claude · 5h");
  assert.equal(hidden(), false);
});

test("拡大操作は一覧の描画世代を変えず、更新と詳細遷移で状態を整合させる", async () => {
  const h = harness();
  h.model.start();
  h.request(0).resolve(snapshot());
  await setImmediate();
  const revision = h.state.revision;
  h.model.actions.focusPanel("work");
  assert.equal(h.state.preview?.route, "work");
  assert.equal(h.state.revision, revision);
  assert.equal(h.requests.length, 1);
  const notifications = h.observed.length;
  h.model.actions.focusPanel("work");
  assert.equal(h.observed.length, notifications);
  h.tick();
  h.request(1).resolve(snapshot("updated"));
  await setImmediate();
  assert.equal(h.state.preview?.route, "work");
  assert.ok(h.state.revision > revision);
  h.model.actions.navigate("#detail/work");
  assert.equal(h.state.preview, undefined);
  h.model.actions.navigate("#overview");
  assert.equal(h.state.preview, undefined);
  h.model.actions.focusPanel("quality");
  h.model.actions.focusPanel(undefined);
  assert.equal(h.state.preview, undefined);
  h.model.dispose();
  h.model.actions.focusPanel("work");
  assert.equal(h.state.preview, undefined);
});
