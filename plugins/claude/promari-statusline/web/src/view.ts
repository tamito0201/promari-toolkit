import type { DashboardVM } from "./presentation.ts";
import type { DashboardActions } from "./view-model.ts";
import { renderInstrument, renderGauge } from "./visualization-view.ts";
import { createFocusPreview } from "./focus-view.ts";
import { byId, el, link } from "./dom.ts";
import {
  renderStat,
  renderPanel,
  renderMetric,
  renderSource,
  renderCard,
} from "./components.ts";

// View は値の反映とイベントの転送だけを担当し、状態の判断は ViewModel に委ねる。
export function createDashboardView(actions: DashboardActions): {
  render(state: DashboardVM): void;
  dispose(): void;
} {
  const subscriptions = new AbortController();
  const options = { signal: subscriptions.signal };
  const preview = createFocusPreview(actions);
  let lastRoute: string | undefined;
  let latest: DashboardVM | undefined;

  function render(data: DashboardVM, force = false): void {
    if (!force && latest?.revision === data.revision) {
      latest = data;
      preview.render(data.preview);
      return;
    }
    const activeMain =
      document.activeElement instanceof HTMLElement &&
      byId("main").contains(document.activeElement)
        ? document.activeElement
        : undefined;
    const focusedSeries = activeMain?.dataset["series"];
    const focusedPreview = activeMain?.dataset["preview"];
    const activeChart = activeMain?.matches(".chart-wrap")
      ? activeMain
      : undefined;
    const chartFocus = activeChart
      ? {
          route: activeChart.dataset["chartRoute"],
          at: activeChart.dataset["selectedAt"],
          name: activeChart.dataset["selectedName"],
        }
      : undefined;
    const focusedHeat = activeMain?.dataset["heatCell"];
    latest = data;
    const c = data.controls;
    (byId("refresh") as HTMLButtonElement).disabled = c.busy;
    byId("pause").setAttribute("aria-pressed", String(c.paused));
    byId("pause").textContent = c.pauseText;
    (byId("range") as HTMLSelectElement).value = String(c.rangeMinutes);
    const search = byId("search") as HTMLInputElement;
    if (search.value !== c.query) search.value = c.query;
    byId("alerts-only").setAttribute("aria-pressed", String(c.onlyAlerts));
    byId("fullscreen").title = c.fullscreenTitle;
    byId("fullscreen").setAttribute("aria-label", c.fullscreenLabel);
    byId("status").dataset["state"] = data.status.kind;
    byId("status-text").textContent = data.status.text;
    byId("freshness").textContent = data.freshness;
    byId("freshness").title = data.freshness;
    byId("freshness").className = data.freshnessClass;
    byId("detail-freshness").textContent = data.detailFreshness;
    byId("updated").textContent = data.updated;
    byId("version").textContent = data.version;
    byId("overview-page").hidden = !data.overview;
    byId("detail-page").hidden = data.overview;
    byId("breadcrumb-current").textContent = data.breadcrumb;
    document.title = data.title;
    byId("summary").replaceChildren(...data.summary.map(renderStat));
    byId("overview").replaceChildren(
      ...data.panels.map((panel) => renderPanel(panel, window.innerWidth)),
    );
    byId("category-strip").replaceChildren(
      ...data.categories.map((c) =>
        link(c.label, c.href, c.alarm ? "alert" : ""),
      ),
      link("全指標 ↗", "#detail/all", "category-all"),
    );
    drawDetail(data);
    preview.render(data.preview);
    if (focusedPreview) {
      [
        ...byId("overview").querySelectorAll<HTMLButtonElement>(
          "[data-preview]",
        ),
      ]
        .find((button) => button.dataset["preview"] === focusedPreview)
        ?.focus({ preventScroll: true });
    }
    if (focusedSeries) {
      const page = byId(data.overview ? "overview-page" : "detail-page");
      [...page.querySelectorAll<HTMLButtonElement>("[data-series]")]
        .find((button) => button.dataset["series"] === focusedSeries)
        ?.focus({ preventScroll: true });
    }
    if (focusedHeat && lastRoute === data.hash) {
      const page = byId(data.overview ? "overview-page" : "detail-page");
      [...page.querySelectorAll<HTMLButtonElement>("[data-heat-cell]")]
        .find((button) => button.dataset["heatCell"] === focusedHeat)
        ?.focus({ preventScroll: true });
    }
    if (chartFocus && lastRoute === data.hash) {
      const page = byId(data.overview ? "overview-page" : "detail-page");
      const chart = [...page.querySelectorAll<HTMLElement>(".chart-wrap")].find(
        (node) => node.dataset["chartRoute"] === chartFocus.route,
      );
      chart?.focus({ preventScroll: true });
      chart?.dispatchEvent(
        new CustomEvent("restore-observation", { detail: chartFocus }),
      );
    }
    if (lastRoute !== data.hash) {
      window.scrollTo(0, 0);
      if (!data.overview) byId("detail-title").focus({ preventScroll: true });
      lastRoute = data.hash;
    }
  }
  function drawDetail(data: DashboardVM): void {
    const detail = data.detail;
    byId("detail-title").textContent = detail.title;
    byId("detail-description").textContent = detail.description;
    byId("detail-charts").replaceChildren(
      ...detail.charts.map((chart) =>
        renderPanel(chart, window.innerWidth, true),
      ),
    );
    const nodes: HTMLElement[] = [];
    if (detail.measurements.length)
      nodes.push(
        el(
          "section",
          "measurement-details",
          el("h3", "", "集計値と算出根拠"),
          el(
            "div",
            "measurement-cards",
            ...detail.measurements.map((metric) =>
              el(
                "article",
                "measurement-card",
                renderMetric(metric),
                metric.visual
                  ? metric.visual.percentage
                    ? renderGauge(metric.visual)
                    : renderInstrument(metric.visual)
                  : null,
                el("p", "", metric.explanation),
                renderSource(metric),
                metric.key
                  ? link(
                      "指標の詳細 ↗",
                      `#metric/${metric.key}`,
                      "measurement-link",
                    )
                  : null,
              ),
            ),
          ),
        ),
      );
    for (const band of detail.bands)
      nodes.push(
        el(
          "section",
          "band",
          el(
            "header",
            "band-head",
            el("h3", "", band.name),
            el("span", "band-description", band.question),
            el("span", "band-count", `${band.cards.length} categories`),
          ),
          el("div", "cards", ...band.cards.map(renderCard)),
        ),
      );
    byId("board").replaceChildren(
      ...(nodes.length ? nodes : [el("p", "no-results", detail.emptyText)]),
    );
  }
  document.addEventListener(
    "click",
    (event) => {
      if (!(event.target instanceof Element)) return;
      const name =
        event.target.closest<HTMLElement>("[data-series]")?.dataset["series"];
      if (name) actions.toggleSeries(name);
    },
    options,
  );
  byId("refresh").addEventListener(
    "click",
    () => {
      void actions.refresh();
    },
    options,
  );
  byId("pause").addEventListener("click", () => actions.togglePause(), options);
  byId("range").addEventListener(
    "change",
    (event) =>
      actions.setRange(Number((event.target as HTMLSelectElement).value)),
    options,
  );
  byId("search").addEventListener(
    "input",
    (event) => actions.search((event.target as HTMLInputElement).value),
    options,
  );
  byId("alerts-only").addEventListener(
    "click",
    () => actions.toggleAlerts(),
    options,
  );
  byId("fullscreen").addEventListener(
    "click",
    () => {
      void actions.toggleFullscreen();
    },
    options,
  );
  byId("overview").addEventListener(
    "click",
    (event) => {
      if (
        !(event.target instanceof Element) ||
        event.target.closest("a,button,input,select")
      )
        return;
      const route =
        event.target.closest<HTMLElement>("[data-panel]")?.dataset["panel"];
      if (route) actions.navigate(`#detail/${route}`);
    },
    options,
  );
  window.addEventListener(
    "resize",
    () => {
      if (latest) render(latest, true);
    },
    options,
  );
  return {
    render,
    dispose() {
      subscriptions.abort();
      preview.dispose();
      latest = undefined;
    },
  };
}
