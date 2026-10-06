import assert from "node:assert/strict";
import { test } from "node:test";
import {
  browserScheduler,
  browserNavigation,
  browserVisibility,
  browserFullscreen,
} from "./browser.ts";

test("ブラウザのタイマーを指定時間で予約し、解除できる", () => {
  const pending = new Map<number, () => void>();
  let delay = 0,
    calls = 0;
  const scheduler = browserScheduler({
    setTimeout(action: () => void, milliseconds: number) {
      delay = milliseconds;
      pending.set(1, action);
      return 1;
    },
    clearTimeout(id: number) {
      pending.delete(id);
    },
  } as unknown as Window);
  const cancel = scheduler.after(5_000, () => {
    calls++;
  });
  assert.equal(delay, 5_000);
  pending.get(1)?.();
  assert.equal(calls, 1);
  cancel();
  assert.equal(pending.size, 0);
});

test("ハッシュ変更をポートに伝え、解除後は通知しない", () => {
  const host = Object.assign(new EventTarget(), {
    location: { hash: "#overview" },
  });
  const navigation = browserNavigation(host as unknown as Window);
  const seen: string[] = [];
  const unsubscribe = navigation.subscribe((hash) => seen.push(hash));
  assert.equal(navigation.current(), "#overview");
  navigation.go("#detail/all");
  host.dispatchEvent(new Event("hashchange"));
  assert.deepEqual(seen, ["#detail/all"]);
  unsubscribe();
  host.dispatchEvent(new Event("hashchange"));
  assert.equal(seen.length, 1);
});

test("タブの表示・非表示を反転させず伝え、解除後は通知しない", () => {
  const doc = Object.assign(new EventTarget(), { hidden: true });
  const visibility = browserVisibility(doc as unknown as Document);
  const seen: boolean[] = [];
  assert.equal(visibility.current(), false);
  const unsubscribe = visibility.subscribe((visible) => seen.push(visible));
  doc.hidden = false;
  doc.dispatchEvent(new Event("visibilitychange"));
  assert.deepEqual(seen, [true]);
  unsubscribe();
  doc.dispatchEvent(new Event("visibilitychange"));
  assert.equal(seen.length, 1);
});

test("全画面表示の開始・終了とブラウザ側の変更を伝える", async () => {
  let enters = 0,
    exits = 0;
  const doc = Object.assign(new EventTarget(), {
    fullscreenElement: null as object | null,
    documentElement: {
      async requestFullscreen() {
        enters++;
        doc.fullscreenElement = {};
        doc.dispatchEvent(new Event("fullscreenchange"));
      },
    },
    async exitFullscreen() {
      exits++;
      doc.fullscreenElement = null;
      doc.dispatchEvent(new Event("fullscreenchange"));
    },
  });
  const fullscreen = browserFullscreen(doc as unknown as Document);
  const seen: boolean[] = [];
  const unsubscribe = fullscreen.subscribe((active) => seen.push(active));
  assert.equal(fullscreen.current(), false);
  await fullscreen.toggle();
  assert.equal(fullscreen.current(), true);
  await fullscreen.toggle();
  assert.equal(fullscreen.current(), false);
  assert.deepEqual([enters, exits], [1, 1]);
  assert.deepEqual(seen, [true, false]);
  unsubscribe();
  doc.dispatchEvent(new Event("fullscreenchange"));
  assert.equal(seen.length, 2);
});
