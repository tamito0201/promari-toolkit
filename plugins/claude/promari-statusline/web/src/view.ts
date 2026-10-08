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
    if (data.overview) fitOverview();
    if (lastRoute !== data.hash) {
      window.scrollTo(0, 0);
      if (!data.overview) byId("detail-title").focus({ preventScroll: true });
      lastRoute = data.hash;
    }
  }
  // 概観は1画面に収める。縮小は zoom ではなく transform で行う——zoom は縮小側の
  // レイアウト幅が flex/grid の親へ素の値のまま伝わる実装があり、ヘッダーとフッターが
  // 画面の外へ引き伸ばされる（2026-10-08 実機で確認）。transform は内箱
  // (#overview-fit) に掛け、外箱 (#overview-page) を縮小後の実寸へ切り抜く——
  // 負のマージン方式は transform 元のレイアウト箱がスクロール領域に残る。
  // 縦横の両方を監視し、はみ出した比率ぶん縮め、余るときは拡大して埋める
  // （縮小の下限0.75倍・拡大の上限1.5倍＝2026-10-08「もっと拡大していい」指示）。
  // 幅が変わると内部の列数も変わって高さが動くため、3回まで反復して寄せる。
  function fitOverview(): void {
    const page = byId("overview-page");
    const fit = byId("overview-fit");
    const root = document.documentElement;
    fit.style.removeProperty("transform");
    fit.style.removeProperty("width");
    // 素の内容の高さを測るため、CSS の 100dvh 最小値を一時的に外す
    fit.style.minHeight = "auto";
    page.style.removeProperty("height");
    if (root.clientHeight === 0 || root.clientWidth === 0) {
      fit.style.removeProperty("min-height");
      return;
    }
    // scrollHeight はビューポート未満にならず、不足（拡大の余地）を検知できない。
    // 実際の使用高さは最下部要素＝フッターの下端で測る。
    const footer = document.querySelector("footer");
    const measure = (k: number): number => {
      fit.style.width = `${100 / k}%`;
      fit.style.transformOrigin = "0 0";
      fit.style.transform = `scale(${k})`;
      page.style.height = `${fit.getBoundingClientRect().height}px`;
      const used = footer
        ? footer.getBoundingClientRect().bottom + window.scrollY
        : root.scrollHeight;
      const vertical = used / root.clientHeight;
      // scrollWidth はビューポート未満にならない（常に1以上）。横は超過の
      // ときだけ採用する（そのまま max に入れると拡大の余地を 1.0 が覆い隠す）。
      const horizontal = root.scrollWidth / root.clientWidth;
      return horizontal > 1.002 ? Math.max(vertical, horizontal) : vertical;
    };
    // 内容の高さは内部列数の段差で跳ぶため、比率の追い込みは発振して
    // 収束しない（0.85→1.10→0.95→1.06 を実測）。描画高さは倍率に対して
    // 単調なので、二分法で「収まる最大の倍率」を探す。
    let lo = 0.75;
    let hi = 1.5;
    if (measure(hi) <= 1.002) {
      lo = hi;
    } else if (measure(lo) > 1.002) {
      hi = lo;
    } else {
      for (let i = 0; i < 6; i += 1) {
        const mid = (lo + hi) / 2;
        if (measure(mid) <= 1.002) lo = mid;
        else hi = mid;
      }
    }
    const landed = measure(lo);
    // 列数の段差で「収まる最大の倍率」でも下に薄く余ることがある。
    // 残りは min-height で頁へ戻し、行間へ等配する（反復中に 100dvh を
    // 混ぜると測定が汚染されるため、確定後にだけ掛ける——実測）。
    if (landed < 0.998) {
      fit.style.minHeight = `calc((100dvh - 89px) / ${lo})`;
      page.style.height = `${fit.getBoundingClientRect().height}px`;
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
