import type {
  Dispose,
  SnapshotRepository,
  Scheduler,
  Navigation,
  Visibility,
  Fullscreen,
} from "./ports.ts";
import {
  presentDashboard,
  type DashboardState,
  type DashboardVM,
} from "./presentation.ts";

const REFRESH_MS = 5_000;
const RANGES = [15, 60, 180] as const;

export interface DashboardActions {
  refresh(): Promise<void>;
  togglePause(): void;
  setRange(minutes: number): void;
  search(query: string): void;
  toggleAlerts(): void;
  toggleSeries(name: string): void;
  focusPanel(route: string | undefined): void;
  navigate(hash: string): void;
  toggleFullscreen(): Promise<void>;
}
export interface DashboardViewModel {
  readonly actions: DashboardActions;
  subscribe(listener: (state: DashboardVM) => void): Dispose;
  start(): void;
  dispose(): void;
}
export interface DashboardDependencies {
  readonly repository: SnapshotRepository;
  readonly scheduler: Scheduler;
  readonly navigation: Navigation;
  readonly visibility: Visibility;
  readonly fullscreen: Fullscreen;
}

// 取得と状態遷移だけを担当する。DOM・fetch・ブラウザのタイマーはポート越しに利用する。
export function createDashboardViewModel(
  deps: DashboardDependencies,
): DashboardViewModel {
  let state: DashboardState = {
    snapshot: undefined,
    hash: deps.navigation.current(),
    rangeMinutes: 60,
    query: "",
    onlyAlerts: false,
    paused: false,
    busy: false,
    offline: false,
    visible: deps.visibility.current(),
    fullscreen: deps.fullscreen.current(),
    fullscreenFailed: false,
    hiddenSeries: [],
    revision: 0,
  };
  const listeners = new Set<(state: DashboardVM) => void>();
  const subscriptions: Dispose[] = [];
  let started = false,
    disposed = false,
    generation = 0,
    fullscreenBusy = false;
  let cancelTimer: Dispose | undefined;
  let request: AbortController | undefined;

  function emit(): void {
    const view = presentDashboard(state);
    for (const listener of listeners) listener(view);
  }
  function update(change: Partial<DashboardState>): void {
    state = { ...state, ...change, revision: (state.revision ?? 0) + 1 };
    emit();
  }
  function stopTimer(): void {
    cancelTimer?.();
    cancelTimer = undefined;
  }
  function cancelRequest(): void {
    generation++;
    request?.abort();
    request = undefined;
    state = { ...state, busy: false };
  }
  function schedule(): void {
    stopTimer();
    if (started && !disposed && state.visible && !state.paused) {
      cancelTimer = deps.scheduler.after(REFRESH_MS, () => {
        void refresh();
      });
    }
  }
  async function refresh(): Promise<void> {
    if (!started || disposed || state.busy) return;
    stopTimer();
    const current = ++generation;
    const controller = new AbortController();
    request = controller;
    update({ busy: true });
    try {
      const snapshot = await deps.repository.load(controller.signal);
      if (disposed || current !== generation) return;
      state = { ...state, snapshot, offline: false };
    } catch {
      if (disposed || current !== generation) return;
      state = { ...state, offline: true };
    } finally {
      // キャンセルを無視して完了する取得元でも、新しい取得の状態を上書きしない。
      if (!disposed && current === generation) {
        request = undefined;
        update({ busy: false });
        schedule();
      }
    }
  }
  const actions: DashboardActions = {
    refresh,
    togglePause() {
      if (disposed) return;
      stopTimer();
      cancelRequest();
      update({ paused: !state.paused });
      if (!state.paused && state.visible) void refresh();
    },
    setRange(minutes) {
      if (!disposed && RANGES.some((range) => range === minutes))
        update({ rangeMinutes: minutes });
    },
    search(query) {
      if (!disposed) update({ query });
    },
    toggleAlerts() {
      if (!disposed) update({ onlyAlerts: !state.onlyAlerts });
    },
    toggleSeries(name) {
      if (disposed) return;
      const hidden = state.hiddenSeries ?? [];
      update({
        hiddenSeries: hidden.includes(name)
          ? hidden.filter((item) => item !== name)
          : [...hidden, name],
      });
    },
    focusPanel(route) {
      if (disposed || route === state.focusedPanel) return;
      state = { ...state, focusedPanel: route };
      emit();
    },
    navigate(hash) {
      if (!disposed) deps.navigation.go(hash);
    },
    async toggleFullscreen() {
      if (disposed || fullscreenBusy) return;
      fullscreenBusy = true;
      try {
        await deps.fullscreen.toggle();
        if (!disposed)
          update({
            fullscreen: deps.fullscreen.current(),
            fullscreenFailed: false,
          });
      } catch {
        if (!disposed) update({ fullscreenFailed: true });
      } finally {
        fullscreenBusy = false;
      }
    },
  };
  return {
    actions,
    subscribe(listener) {
      if (disposed) return () => {};
      listeners.add(listener);
      listener(presentDashboard(state));
      return () => {
        listeners.delete(listener);
      };
    },
    start() {
      if (started || disposed) return;
      started = true;
      subscriptions.push(
        deps.navigation.subscribe((hash) => {
          if (hash !== state.hash)
            update({
              hash,
              query: "",
              onlyAlerts: false,
              focusedPanel: undefined,
            });
        }),
        deps.visibility.subscribe((visible) => {
          stopTimer();
          if (!visible) cancelRequest();
          update({ visible });
          if (visible && !state.paused) void refresh();
        }),
        deps.fullscreen.subscribe((fullscreen) =>
          update({ fullscreen, fullscreenFailed: false }),
        ),
      );
      if (state.visible && !state.paused) void refresh();
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      stopTimer();
      cancelRequest();
      for (const unsubscribe of subscriptions) unsubscribe();
      listeners.clear();
    },
  };
}
