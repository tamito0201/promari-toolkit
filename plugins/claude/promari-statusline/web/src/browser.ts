import type {
  Dispose,
  Scheduler,
  Navigation,
  Visibility,
  Fullscreen,
} from "./ports.ts";

// ブラウザ API の実装をここに集める。各ポートの購読解除は呼び出し側が所有する。
function listen(
  target: EventTarget,
  event: string,
  action: () => void,
): Dispose {
  target.addEventListener(event, action);
  return () => target.removeEventListener(event, action);
}
export function browserScheduler(win: Window): Scheduler {
  return {
    after(delay, action) {
      const timer = win.setTimeout(action, delay);
      return () => win.clearTimeout(timer);
    },
  };
}
export function browserNavigation(win: Window): Navigation {
  return {
    current: () => win.location.hash,
    go: (hash) => {
      win.location.hash = hash;
    },
    subscribe: (listener) =>
      listen(win, "hashchange", () => listener(win.location.hash)),
  };
}
export function browserVisibility(doc: Document): Visibility {
  return {
    current: () => !doc.hidden,
    subscribe: (listener) =>
      listen(doc, "visibilitychange", () => listener(!doc.hidden)),
  };
}
export function browserFullscreen(doc: Document): Fullscreen {
  return {
    current: () => doc.fullscreenElement !== null,
    async toggle() {
      if (doc.fullscreenElement) await doc.exitFullscreen();
      else await doc.documentElement.requestFullscreen();
    },
    subscribe: (listener) =>
      listen(doc, "fullscreenchange", () =>
        listener(doc.fullscreenElement !== null),
      ),
  };
}
