import type { ChartVM, SeriesVM } from "./presentation.ts";
import { clock, dateTime } from "./format.ts";
import { el, svg, COLORS } from "./dom.ts";
import { renderHeatmap } from "./visualization-view.ts";
import { observations, nearestObservation, cursorIndex } from "./chart-math.ts";

const THRESHOLD_PERCENT = 90;
const PERCENT_SCALE = 100;
const GRID_DIVISIONS = 4;
const compact = (value: number): string =>
  Intl.NumberFormat("en-US", {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(value);
function legend(series: SeriesVM): HTMLElement {
  const dot = el("span", "legend-dot");
  dot.style.setProperty("--series-color", COLORS[series.color]);
  const button = el("button", "legend-item", dot, series.name);
  button.type = "button";
  button.dataset["series"] = series.name;
  button.setAttribute("aria-pressed", String(!series.hidden));
  button.title = `${series.name} の表示を切り替え`;
  return button;
}
function chartWidth(
  route: string,
  detail: boolean,
  viewportWidth: number,
): number {
  const available = viewportWidth - (viewportWidth <= 600 ? 48 : 70);
  if (detail)
    return Math.max(220, viewportWidth <= 1100 ? available : available / 2);
  if (viewportWidth <= 600 || (viewportWidth <= 1100 && route === "limits"))
    return Math.max(220, available);
  if (viewportWidth <= 1100) return Math.max(220, available / 2);
  return Math.max(220, (available * (route === "limits" ? 5 : 4)) / 12);
}
// SVG の座標・入力操作だけを担当し、観測値や表示系列の状態は変更しない。
export function renderChart(
  data: ChartVM,
  detail: boolean,
  viewportWidth: number,
): HTMLElement[] {
  const { title, series, from, until, unit, maximum, route } = data;
  const points = observations(series);
  const max =
    Math.max(
      maximum ?? 1,
      ...series.flatMap((s) => s.points.map((p) => p.value)),
    ) * (maximum ? 1 : 1.15);
  const left = 39,
    right = 10,
    top = 14,
    bottom = 26;
  // 推定値で先に描き、枠の実寸が分かったら実寸で描き直す。
  // viewBox を表示寸法と 1:1 に保ち、軸の数字や点が縦横に引き伸ばされないようにする。
  let width = chartWidth(route, detail, viewportWidth),
    height = detail ? 220 : 118;
  const plotWidth = (): number => width - left - right;
  const x = (time: number): number =>
    left + ((time - from) / Math.max(1, until - from)) * plotWidth();
  const y = (value: number): number =>
    top + (1 - value / max) * (height - top - bottom);
  const graphic = svg("svg", {
    class: "chart",
    preserveAspectRatio: "none",
    "aria-hidden": "true",
  });
  const crosshair = svg("line", {
    class: "chart-crosshair",
    visibility: "hidden",
  });
  const marker = svg("circle", {
    r: 4,
    class: "chart-marker",
    visibility: "hidden",
  });
  function draw(): void {
    graphic.setAttribute("viewBox", `0 0 ${width} ${height}`);
    graphic.replaceChildren();
    for (let i = 0; i <= GRID_DIVISIONS; i++) {
      const value = (max * i) / GRID_DIVISIONS,
        time = from + ((until - from) * i) / GRID_DIVISIONS;
      graphic.append(
        svg("line", {
          x1: left,
          x2: width - right,
          y1: y(value),
          y2: y(value),
          class: "grid-line",
        }),
        svg(
          "text",
          { x: left - 8, y: y(value) + 3, "text-anchor": "end" },
          maximum ? `${Math.round(value)}%` : compact(value),
        ),
        svg(
          "text",
          {
            x: x(time),
            y: height - 6,
            "text-anchor":
              i === 0 ? "start" : i === GRID_DIVISIONS ? "end" : "middle",
          },
          clock(time),
        ),
      );
    }
    if (maximum === PERCENT_SCALE)
      graphic.append(
        svg("line", {
          x1: left,
          x2: width - right,
          y1: y(THRESHOLD_PERCENT),
          y2: y(THRESHOLD_PERCENT),
          class: "threshold",
        }),
      );
    for (const [index, s] of series.entries()) {
      if (s.hidden) continue;
      // 実測点の間だけを結び、最後の観測を現在まで延長しない。
      const path = s.points
        .map(
          (p, i) =>
            `${i === 0 ? "M" : "L"}${x(p.at).toFixed(2)},${y(p.value).toFixed(2)}`,
        )
        .join(" ");
      const first = s.points[0],
        last = s.points.at(-1);
      if (first && last && s.points.length > 1) {
        graphic.append(
          svg("path", {
            d: `${path} L${x(last.at)},${y(0)} L${x(first.at)},${y(0)} Z`,
            fill: COLORS[s.color],
            opacity: 0.09,
          }),
          svg("path", {
            d: path,
            stroke: COLORS[s.color],
            class: "series-line",
            "stroke-dasharray": index % 2 ? "5 3" : "none",
          }),
        );
      }
      for (const point of s.points) {
        const dot = svg("circle", {
          cx: x(point.at),
          cy: y(point.value),
          r: point === last ? 3.2 : s.points.length > 40 ? 1.2 : 2,
          fill: COLORS[s.color],
          class: "chart-point",
        });
        dot.append(
          svg(
            "title",
            {},
            `${s.name} · ${dateTime(new Date(point.at).toISOString())} · ${point.value}${unit}`,
          ),
        );
        graphic.append(dot);
      }
    }
    crosshair.setAttribute("x1", String(left));
    crosshair.setAttribute("x2", String(left));
    crosshair.setAttribute("y1", String(top));
    crosshair.setAttribute("y2", String(height - bottom));
    graphic.append(crosshair, marker);
  }
  draw();
  const wrap = el("div", "chart-wrap", graphic);
  const tooltip = el("div", "chart-tooltip");
  tooltip.hidden = true;
  tooltip.setAttribute("role", "status");
  tooltip.setAttribute("aria-live", "polite");
  wrap.append(tooltip);
  wrap.tabIndex = 0;
  wrap.dataset["chartRoute"] = route;
  wrap.setAttribute("role", "group");
  wrap.setAttribute(
    "aria-label",
    `${title}。${points.length}観測点。左右キーで実測値を確認、HomeとEndで先頭と末尾へ移動。`,
  );
  let selected = points.length - 1;
  function select(index: number): void {
    const point = points[index];
    if (!point) return;
    selected = index;
    wrap.dataset["selectedAt"] = String(point.at);
    wrap.dataset["selectedName"] = point.name;
    const px = x(point.at);
    crosshair.setAttribute("x1", String(px));
    crosshair.setAttribute("x2", String(px));
    crosshair.setAttribute("visibility", "visible");
    marker.setAttribute("cx", String(px));
    marker.setAttribute("cy", String(y(point.value)));
    marker.setAttribute("fill", COLORS[point.color]);
    marker.setAttribute("visibility", "visible");
    // 同時刻の観測だけを列挙する。別時刻の値を同時刻の値として表示しない。
    const rows = points.filter((item) => item.at === point.at);
    tooltip.replaceChildren(
      el("time", "", dateTime(new Date(point.at).toISOString())),
      ...rows.map((item) =>
        el(
          "div",
          "tooltip-row",
          el("span", "", item.name),
          el(
            "b",
            "",
            `${Intl.NumberFormat("en-US", { maximumFractionDigits: 3 }).format(item.value)}${unit}`,
          ),
        ),
      ),
    );
    tooltip.hidden = false;
    tooltip.style.left = px > width / 2 ? "auto" : "12%";
    tooltip.style.right = px > width / 2 ? "6%" : "auto";
  }
  function hide(): void {
    tooltip.hidden = true;
    crosshair.setAttribute("visibility", "hidden");
    marker.setAttribute("visibility", "hidden");
  }
  // 表示寸法の変化（拡大枠・画面回転・ウィンドウ幅）に追従して実寸で描き直す。
  if (typeof ResizeObserver !== "undefined") {
    const observer = new ResizeObserver(([entry]) => {
      if (!wrap.isConnected) return observer.disconnect();
      const box = entry?.contentRect;
      if (!box || box.width < 1 || box.height < 1) return;
      const nextWidth = Math.round(box.width),
        nextHeight = Math.round(box.height);
      if (nextWidth === width && nextHeight === height) return;
      width = nextWidth;
      height = nextHeight;
      draw();
      if (!tooltip.hidden) select(selected);
    });
    observer.observe(wrap);
  }
  wrap.addEventListener("restore-observation", (event) => {
    const { at, name } = (
      event as CustomEvent<{ at: string | undefined; name: string | undefined }>
    ).detail;
    const index = points.findIndex(
      (point) => String(point.at) === at && point.name === name,
    );
    select(index < 0 ? points.length - 1 : index);
  });
  wrap.addEventListener("pointermove", (event) => {
    const bounds = wrap.getBoundingClientRect();
    const px =
      ((event.clientX - bounds.left) / Math.max(1, bounds.width)) * width;
    select(
      nearestObservation(
        points,
        from + ((px - left) / plotWidth()) * (until - from),
      ),
    );
  });
  wrap.addEventListener("pointerdown", (event) => {
    event.stopPropagation();
    wrap.focus({ preventScroll: true });
  });
  wrap.addEventListener("click", (event) => event.stopPropagation());
  wrap.addEventListener("pointerleave", hide);
  wrap.addEventListener("blur", hide);
  wrap.addEventListener("focus", () => select(selected));
  wrap.addEventListener("keydown", (event) => {
    if (
      !["ArrowLeft", "ArrowRight", "Home", "End", "Escape"].includes(event.key)
    )
      return;
    event.preventDefault();
    if (event.key === "Escape") hide();
    else select(cursorIndex(selected, event.key, points.length));
  });
  if (!points.length)
    wrap.append(
      el(
        "div",
        "chart-empty",
        el(
          "b",
          "",
          series.every((s) => s.hidden)
            ? "系列を選択してください"
            : "この期間の記録はありません",
        ),
        "上の凡例で表示する系列を切り替え",
      ),
    );
  const last = points.at(-1)?.at;
  return [
    el("div", "chart-legend", ...series.map(legend)),
    wrap,
    ...(data.heatmap ? [renderHeatmap(data.heatmap)] : []),
    el(
      "div",
      "chart-caption",
      el("span", "", `${points.length} 実測点 · カーソル / ← → で確認`),
      el(
        "span",
        "",
        last === undefined ? "履歴待ち" : `最終 ${clock(last, { seconds: true })}`,
      ),
    ),
  ];
}
