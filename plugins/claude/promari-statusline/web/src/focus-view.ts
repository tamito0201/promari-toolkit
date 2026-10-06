import type { PreviewVM } from "./presentation.ts";
import type { DashboardActions } from "./view-model.ts";
import { renderPanel, renderMetric, renderCard, renderSource } from "./components.ts";
import { byId, el, link } from "./dom.ts";

const OPEN_DELAY_MS = 220;
const CLOSE_DELAY_MS = 280;
const PANEL_HORIZONTAL_INSET = 16;

// フォーカスとホバーの受け渡しだけを担当する。拡大する図・指標の選択は ViewModel が行う。
export function createFocusPreview(
  actions: Pick<DashboardActions, "focusPanel">,
): {
  render(data: PreviewVM | undefined): void;
  dispose(): void;
} {
  const controller = new AbortController();
  const options = { signal: controller.signal };
  const title = el("h2");
  title.id = "focus-title";
  const close = el("button", "button", "閉じる ×");
  close.type = "button";
  close.setAttribute("aria-label", "拡大表示を閉じる（Escape）");
  const full = link("詳細ページへ ↗", "#overview", "button");
  const freshness = el("p", "focus-freshness");
  const description = el("p", "focus-description");
  const contents = el("div", "focus-content");
  const root = el(
    "aside",
    "focus-preview",
    el("header", "focus-head", title, full, close),
    description,
    freshness,
    contents,
  );
  root.id = "focus-preview";
  root.hidden = true;
  root.setAttribute("role", "region");
  root.setAttribute("aria-labelledby", "focus-title");
  document.body.append(root);
  const overview = byId("overview");
  let route: string | undefined;
  let suppressed: string | undefined;
  let openTimer: number | undefined;
  let closeTimer: number | undefined;

  const panelRoute = (target: EventTarget | null): string | undefined =>
    target instanceof Element
      ? target.closest<HTMLElement>("#overview [data-panel]")?.dataset["panel"]
      : undefined;
  const inside = (target: EventTarget | null): boolean =>
    target instanceof Node && root.contains(target);
  function cancelTimers(): void {
    window.clearTimeout(openTimer);
    window.clearTimeout(closeTimer);
    openTimer = closeTimer = undefined;
  }
  function dismiss(restore = false): void {
    cancelTimers();
    const previous = route;
    suppressed = previous;
    actions.focusPanel(undefined);
    if (restore)
      [...overview.querySelectorAll<HTMLButtonElement>("[data-preview]")]
        .find((button) => button.dataset["preview"] === previous)
        ?.focus({ preventScroll: true });
  }
  function show(next: string | undefined, delayed: boolean): void {
    cancelTimers();
    if (!next || next === suppressed) return;
    if (delayed)
      openTimer = window.setTimeout(
        () => actions.focusPanel(next),
        OPEN_DELAY_MS,
      );
    else actions.focusPanel(next);
  }
  function scheduleClose(): void {
    window.clearTimeout(openTimer);
    window.clearTimeout(closeTimer);
    closeTimer = window.setTimeout(() => {
      if (!inside(document.activeElement)) actions.focusPanel(undefined);
    }, CLOSE_DELAY_MS);
  }
  overview.addEventListener(
    "pointerover",
    (event) => {
      if (event.pointerType !== "mouse") return;
      const next = panelRoute(event.target);
      if (next === panelRoute(event.relatedTarget)) return;
      if (next !== suppressed) suppressed = undefined;
      show(next, true);
    },
    options,
  );
  overview.addEventListener(
    "pointerout",
    (event) => {
      const previous = panelRoute(event.target);
      if (previous === panelRoute(event.relatedTarget)) return;
      suppressed = undefined;
      if (inside(event.relatedTarget)) cancelTimers();
      else scheduleClose();
    },
    options,
  );
  overview.addEventListener(
    "focusin",
    (event) => {
      const next = panelRoute(event.target);
      if (next !== suppressed) suppressed = undefined;
      show(next, false);
    },
    options,
  );
  overview.addEventListener(
    "focusout",
    (event) => {
      if (
        inside(event.relatedTarget) ||
        panelRoute(event.relatedTarget) === route
      )
        return;
      scheduleClose();
    },
    options,
  );
  overview.addEventListener(
    "click",
    (event) => {
      if (!(event.target instanceof Element)) return;
      const next =
        event.target.closest<HTMLElement>("[data-preview]")?.dataset["preview"];
      if (!next) return;
      suppressed = undefined;
      show(next, false);
      close.focus({ preventScroll: true });
    },
    options,
  );
  root.addEventListener("pointerenter", cancelTimers, options);
  root.addEventListener("pointerleave", scheduleClose, options);
  root.addEventListener("focusin", cancelTimers, options);
  root.addEventListener(
    "focusout",
    (event) => {
      if (
        !inside(event.relatedTarget) &&
        panelRoute(event.relatedTarget) !== route
      )
        scheduleClose();
    },
    options,
  );
  close.addEventListener("click", () => dismiss(true), options);
  document.addEventListener(
    "keydown",
    (event) => {
      if (event.key === "Escape" && !root.hidden) {
        event.preventDefault();
        dismiss(inside(document.activeElement));
      }
    },
    options,
  );
  document.addEventListener(
    "pointerdown",
    (event) => {
      if (
        !root.hidden &&
        !inside(event.target) &&
        panelRoute(event.target) !== route
      )
        dismiss();
    },
    options,
  );
  return {
    render(data) {
      const changed = route !== data?.route;
      route = data?.route;
      root.hidden = !data;
      for (const button of overview.querySelectorAll<HTMLElement>(
        "[data-preview]",
      ))
        button.setAttribute(
          "aria-expanded",
          String(button.dataset["preview"] === route),
        );
      if (!data) {
        contents.replaceChildren();
        return;
      }
      const active =
        document.activeElement instanceof HTMLElement &&
        inside(document.activeElement)
          ? document.activeElement
          : undefined;
      const series = active?.dataset["series"],
        heat = active?.dataset["heatCell"];
      const chart = active?.matches(".chart-wrap")
        ? {
            at: active.dataset["selectedAt"],
            name: active.dataset["selectedName"],
          }
        : undefined;
      title.textContent = data.detail.title;
      description.textContent = data.detail.description;
      freshness.textContent = data.freshness;
      full.href = `#detail/${data.route}`;
      const scrollTop = changed
        ? 0
        : (contents.querySelector(".focus-data")?.scrollTop ?? 0);
      const contentScrollTop = changed ? 0 : contents.scrollTop;
      // 元のステータスも既定で開き、利用者が閉じた場合だけ更新後も閉じたままにする。
      const sourceOpen =
        changed ||
        (contents.querySelector<HTMLDetailsElement>(".focus-source")?.open ??
          true);
      // 算出条件は既定で開き、利用者が閉じた項目だけを更新後も閉じたままにする。
      const collapsedReadings = new Set(
        changed
          ? []
          : [
              ...contents.querySelectorAll<HTMLElement>(
                "[data-explanation]:not([open])",
              ),
            ].map((item) => item.dataset["explanation"]),
      );
      const focusedExplanation =
        active?.closest<HTMLElement>("[data-explanation]")?.dataset[
          "explanation"
        ];
      const focusedReadings = active?.matches(".focus-data");
      const focusedSource = active?.matches(".focus-source > summary");
      const metrics = el(
        "div",
        "focus-measurements",
        ...data.detail.measurements.map((metric) => {
          const explanation = el(
            "details",
            "focus-explanation",
            el("summary", "", "算出条件"),
            el("p", "", metric.explanation),
            renderSource(metric),
          );
          explanation.dataset["explanation"] = metric.key ?? metric.label;
          explanation.open = !collapsedReadings.has(
            explanation.dataset["explanation"],
          );
          const item = el(
            "article",
            "focus-reading",
            renderMetric(metric),
            explanation,
          );
          return item;
        }),
      );
      const source = el(
        "details",
        "focus-source",
        el("summary", "", "元のステータスを表示"),
        el(
          "div",
          "cards",
          ...data.detail.bands.flatMap((band) => band.cards.map(renderCard)),
        ),
      );
      source.open = sourceOpen;
      const readings = el(
        "section",
        "focus-data",
        el(
          "h3",
          "focus-data-title",
          `数値・算出条件 · ${data.detail.measurements.length}指標`,
        ),
        metrics,
        source,
      );
      readings.setAttribute("aria-label", "数値と算出条件");
      readings.tabIndex = 0;
      const frame = el("div", "detail-charts focus-chart");
      contents.replaceChildren(frame, readings);
      // SVG の横幅は拡大枠の実幅に合わせ、数値欄の追加で軸の文字を縮小しない。
      frame.append(
        renderPanel(
          data.panel,
          frame.clientWidth + PANEL_HORIZONTAL_INSET,
          true,
        ),
      );
      readings.scrollTop = scrollTop;
      contents.scrollTop = contentScrollTop;
      if (focusedReadings) readings.focus({ preventScroll: true });
      if (focusedSource)
        source.querySelector("summary")?.focus({ preventScroll: true });
      if (focusedExplanation)
        [...metrics.querySelectorAll<HTMLElement>("[data-explanation]")]
          .find((item) => item.dataset["explanation"] === focusedExplanation)
          ?.querySelector("summary")
          ?.focus({ preventScroll: true });
      if (series)
        [...contents.querySelectorAll<HTMLButtonElement>("[data-series]")]
          .find((button) => button.dataset["series"] === series)
          ?.focus({ preventScroll: true });
      if (heat)
        [...contents.querySelectorAll<HTMLButtonElement>("[data-heat-cell]")]
          .find((button) => button.dataset["heatCell"] === heat)
          ?.focus({ preventScroll: true });
      if (chart) {
        const target = contents.querySelector<HTMLElement>(".chart-wrap");
        target?.focus({ preventScroll: true });
        target?.dispatchEvent(
          new CustomEvent("restore-observation", { detail: chart }),
        );
      }
    },
    dispose() {
      cancelTimers();
      controller.abort();
      root.remove();
    },
  };
}
