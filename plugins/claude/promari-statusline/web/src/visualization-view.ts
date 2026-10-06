import type { HeatmapVM, InstrumentVM, VisualVM } from "./visualization.ts";
import { clock, dateTime } from "./format.ts";
import { el, svg, link } from "./dom.ts";

const RADIUS = 29;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;
const HALF_ARC = 180;
const SWEEP = 270;

function meter(item: InstrumentVM, compact = false): HTMLElement {
  const fill = el("i", "instrument-fill");
  fill.style.setProperty("--fraction", String(item.fraction ?? 0));
  const rail = el(
    "div",
    `instrument-rail${item.value === undefined ? " is-missing" : ""}`,
    fill,
  );
  if (item.value !== undefined) {
    const dot = el("span", "instrument-dot");
    dot.style.setProperty("--fraction", String(item.fraction ?? 0));
    rail.append(dot);
  }
  if (item.reference) {
    const marker = el("span", "instrument-reference");
    marker.style.setProperty("--fraction", String(item.reference.fraction));
    marker.title = item.reference.text;
    rail.append(marker);
  }
  const root = el(
    "div",
    `instrument tone-${item.tone}${compact ? " is-compact" : ""}`,
    rail,
  );
  root.dataset["visualMetric"] = item.key;
  root.setAttribute("role", "img");
  root.setAttribute(
    "aria-label",
    `${item.label}: ${item.text}。${item.value === undefined ? "値を表示できません" : `目盛0から${item.maximumText}。${item.basis}`}${item.reference ? `。参考線 ${item.reference.text}` : ""}`,
  );
  if (!compact)
    root.append(
      el(
        "div",
        "instrument-axis",
        el("span", "", "0"),
        el(
          "span",
          "",
          item.value === undefined ? "観測待ち" : item.maximumText,
        ),
      ),
    );
  return root;
}
export function renderInstrument(item: InstrumentVM): HTMLElement {
  return el(
    "div",
    "metric-visual",
    meter(item),
    el(
      "small",
      "instrument-basis",
      `${item.basis}${item.reference ? ` · 参考線 ${item.reference.text}` : ""}`,
    ),
  );
}
export function renderGauge(item: InstrumentVM, small = false): HTMLElement {
  const arc = (CIRCUMFERENCE * SWEEP) / 360;
  const graphic = svg("svg", {
    viewBox: "0 0 76 72",
    class: "gauge-svg",
    "aria-hidden": "true",
  });
  const attrs = {
    cx: 38,
    cy: 36,
    r: RADIUS,
    fill: "none",
    "stroke-width": 5,
    transform: `rotate(${HALF_ARC - (SWEEP - HALF_ARC) / 2} 38 36)`,
  };
  graphic.append(
    svg("circle", {
      ...attrs,
      class: "gauge-track",
      "stroke-dasharray": `${arc} ${CIRCUMFERENCE}`,
      "stroke-linecap": "round",
    }),
  );
  if (item.fraction !== undefined)
    graphic.append(
      svg("circle", {
        ...attrs,
        class: "gauge-fill",
        "stroke-dasharray": `${arc * item.fraction} ${CIRCUMFERENCE}`,
        "stroke-linecap": "round",
      }),
    );
  graphic.append(
    svg(
      "text",
      { x: 38, y: 37, "text-anchor": "middle", class: "gauge-number" },
      item.value === undefined ? "—" : item.text,
    ),
  );
  graphic.append(
    svg(
      "text",
      { x: 38, y: 49, "text-anchor": "middle", class: "gauge-unit" },
      item.value === undefined ? "NO DATA" : `0 – ${item.maximumText}`,
    ),
  );
  const root = el(
    "div",
    `gauge tone-${item.tone}${small ? " is-small" : ""}`,
    graphic,
    el(
      "span",
      "gauge-label",
      item.label,
      item.value === undefined ? el("small", "gauge-missing", item.text) : null,
    ),
  );
  root.dataset["visualMetric"] = item.key;
  root.setAttribute("role", "img");
  root.setAttribute("aria-label", `${item.label}: ${item.text}。${item.basis}`);
  root.title = `${item.label}: ${item.text} · ${item.basis}`;
  return root;
}
function row(item: InstrumentVM, index: number): HTMLElement {
  const label = item.href
    ? link(item.label, item.href, "visual-label")
    : el("span", "visual-label", item.label);
  const root = el(
    "div",
    `visual-row tone-${item.tone}`,
    el(
      "div",
      "visual-row-heading",
      label,
      el(
        "span",
        "visual-value-group",
        el("strong", "visual-value", item.text),
        item.value === undefined
          ? null
          : el("small", "visual-scale", `目盛 ${item.maximumText}`),
      ),
    ),
    meter(item, true),
  );
  root.dataset["known"] = String(item.value !== undefined);
  root.style.setProperty("--series-index", String(index));
  root.title = `${item.label}: ${item.text} · ${item.basis}`;
  return root;
}
function composition(data: VisualVM): HTMLElement {
  const graphic = svg("svg", { viewBox: "0 0 90 90", "aria-hidden": "true" });
  const radius = 35,
    circle = 2 * Math.PI * radius;
  graphic.append(
    svg("circle", {
      cx: 45,
      cy: 45,
      r: radius,
      fill: "none",
      "stroke-width": 8,
      class: "gauge-track",
    }),
  );
  let offset = 0;
  for (const [i, item] of data.items.entries()) {
    if (item.fraction === undefined) continue;
    graphic.append(
      svg("circle", {
        cx: 45,
        cy: 45,
        r: radius,
        fill: "none",
        "stroke-width": 8,
        transform: "rotate(-90 45 45)",
        class: `composition-segment segment-${i}`,
        "stroke-dasharray": `${item.fraction * circle} ${circle}`,
        "stroke-dashoffset": -offset * circle,
      }),
    );
    offset += item.fraction;
  }
  graphic.append(
    svg(
      "text",
      { x: 45, y: 44, "text-anchor": "middle", class: "composition-total" },
      data.total ?? "—",
    ),
    svg(
      "text",
      { x: 45, y: 57, "text-anchor": "middle", class: "gauge-unit" },
      "TOKENS",
    ),
  );
  const ring = el("div", "composition-ring", graphic);
  ring.setAttribute("role", "img");
  ring.setAttribute(
    "aria-label",
    `${data.title}。${data.items.map((item) => `${item.label}: ${item.text}`).join("、")}`,
  );
  return el(
    "div",
    "composition",
    ring,
    el(
      "div",
      "composition-items",
      ...data.items.map((item, i) => {
        const node = row(item, i);
        node.classList.add(`segment-${i}`);
        return node;
      }),
    ),
  );
}
function balance(data: VisualVM): HTMLElement {
  const chart = el("div", "balance-chart");
  for (const [index, item] of data.items.entries()) {
    const bar = el("i");
    bar.style.width = `${(item.fraction ?? 0) * 100}%`;
    const half = el(
      "div",
      `balance-half ${index === 0 ? "added" : "deleted"}${item.value === undefined ? " is-missing" : ""}`,
      bar,
    );
    half.title = `${item.label}: ${item.text}`;
    half.dataset["visualMetric"] = item.key;
    chart.append(half);
  }
  chart.setAttribute("role", "img");
  chart.setAttribute(
    "aria-label",
    data.items.map((item) => `${item.label}: ${item.text}`).join("、"),
  );
  const values = el(
    "div",
    "balance-values",
    ...data.items.map((item, index) =>
      el(
        "div",
        index === 0 ? "tone-good" : "tone-danger",
        el(
          "strong",
          "",
          `${item.value === undefined ? "" : index === 0 ? "+" : "−"}${item.text}`,
        ),
        el("span", "", item.label),
      ),
    ),
  );
  return el(
    "div",
    "balance",
    values,
    chart,
    el(
      "div",
      "balance-axis",
      data.items[0]?.maximumText ?? "",
      el("span", "", "0"),
      data.items[1]?.maximumText ?? "",
    ),
  );
}
const WAFFLE_CELLS = 10;
function columns(data: VisualVM): HTMLElement {
  const chart = el(
    "div",
    "column-chart",
    ...data.items.map((item, index) => {
      const fill = el("i", "column-fill");
      fill.style.height = `${(item.fraction ?? 0) * 100}%`;
      const track = el(
        "div",
        `column-track${item.value === undefined ? " is-missing" : ""}`,
        fill,
      );
      const root = link(
        "",
        item.href ?? "#detail/work",
        `column-item column-${index}`,
      );
      root.append(
        el("strong", "column-value", item.text),
        track,
        el("span", "column-label", item.label),
      );
      root.dataset["visualMetric"] = item.key;
      root.title = `${item.label}: ${item.text} · ${item.basis}`;
      return root;
    }),
  );
  return el(
    "div",
    "columns",
    el("div", "column-scale", `目盛 0–${data.items[0]?.maximumText ?? "—"}`),
    chart,
  );
}
function waffle(data: VisualVM): HTMLElement {
  return el(
    "div",
    "waffle-grid",
    ...data.items.map((item) => {
      const cells = el(
        "div",
        `waffle-cells${item.value === undefined ? " is-missing" : ""}`,
      );
      cells.setAttribute("aria-hidden", "true");
      for (let index = 0; index < WAFFLE_CELLS; index++) {
        const fill = el("i");
        fill.style.width = `${Math.max(0, Math.min(1, (item.fraction ?? 0) * WAFFLE_CELLS - index)) * 100}%`;
        cells.append(el("span", "waffle-cell", fill));
      }
      const root = link(
        "",
        item.href ?? "#detail/quality",
        `waffle-tile tone-${item.tone}`,
      );
      root.append(
        el(
          "div",
          "waffle-heading",
          el("span", "", item.label),
          el("strong", "", item.text),
        ),
        cells,
        el("small", "", `0–${item.maximumText}`),
      );
      root.dataset["visualMetric"] = item.key;
      root.title = `${item.label}: ${item.text} · ${item.basis}`;
      return root;
    }),
  );
}
function timeBand(data: VisualVM): HTMLElement {
  const bar = el("div", "time-band");
  for (const [index, item] of data.items.entries()) {
    const segment = link(
      "",
      item.href ?? "#detail/costs",
      `time-segment segment-${index}`,
    );
    segment.style.width = `${(item.fraction ?? 0) * 100}%`;
    segment.title = `${item.label}: ${item.text}`;
    segment.setAttribute("aria-label", segment.title);
    bar.append(segment);
  }
  if (data.total === undefined) bar.classList.add("is-missing");
  return el(
    "div",
    "time-allocation",
    el(
      "div",
      "time-legend",
      ...data.items.map((item, index) =>
        el(
          "span",
          `segment-${index}`,
          el("i"),
          item.label,
          el("b", "", item.text),
        ),
      ),
    ),
    bar,
  );
}
function quantileRange(data: VisualVM): HTMLElement {
  const width = 170,
    left = 14,
    right = 156;
  const graphic = svg("svg", {
    viewBox: `0 0 ${width} 80`,
    class: "quantile-plot",
    "aria-hidden": "true",
  });
  data.items.forEach((item, index) => {
    const y = 18 + index * 34;
    graphic.append(
      svg("line", {
        x1: left,
        x2: right,
        y1: y,
        y2: y,
        class: "quantile-track",
      }),
    );
    if (item.fraction !== undefined)
      graphic.append(
        svg("circle", {
          cx: left + item.fraction * (right - left),
          cy: y,
          r: 4,
          class: `quantile-point segment-${index}`,
        }),
      );
    graphic.append(
      svg(
        "text",
        { x: left, y: y + 15, class: "quantile-label" },
        `${item.label}  ${item.text}`,
      ),
    );
  });
  const root = el(
    "div",
    "quantile",
    graphic,
    el("small", "", `共通目盛 0–${data.items[0]?.maximumText ?? "—"}`),
  );
  root.setAttribute("role", "img");
  root.setAttribute(
    "aria-label",
    data.items.map((item) => `${item.label}: ${item.text}`).join("、"),
  );
  return root;
}
const RENDERERS: Record<VisualVM["layout"], (data: VisualVM) => HTMLElement> = {
  bars: (data) => el("div", "visual-bars", ...data.items.map(row)),
  lollipop: (data) =>
    el("div", "visual-bars lollipop-bars", ...data.items.map(row)),
  rings: (data) =>
    el(
      "div",
      "gauge-group",
      ...data.items.map((item) => {
        const gauge = renderGauge(item);
        if (!item.href) return gauge;
        const a = link("", item.href);
        a.append(gauge);
        return a;
      }),
    ),
  composition,
  balance,
  columns,
  waffle,
  time: timeBand,
  range: quantileRange,
};
export function renderVisual(data: VisualVM): HTMLElement {
  const root = el(
    "div",
    `visual-block visual-${data.layout}`,
    RENDERERS[data.layout](data),
  );
  root.dataset["visualization"] = data.layout;
  root.title = data.note;
  if (data.note) root.append(el("p", "visual-note", data.note));
  return root;
}
export function renderHeatmap(data: HeatmapVM): HTMLElement {
  const root = el("div", "rate-heatmap");
  root.setAttribute(
    "aria-label",
    "時間帯別レート使用率。区間内の実測最大値。斜線は観測なし。",
  );
  for (const row of data.rows) {
    const cells = el("div", "heatmap-cells");
    const buttons = row.cells.map((cell, index) => {
      const button = el(
        "button",
        `heat-cell${cell.peak === undefined ? " is-missing" : ""}`,
      );
      button.type = "button";
      button.dataset["heatCell"] = `${row.label}/${index}`;
      button.tabIndex = index === 0 ? 0 : -1;
      const peak = cell.peak;
      button.style.setProperty(
        "--heat",
        String(peak === undefined ? 0 : Math.max(0, Math.min(1, peak / 100))),
      );
      if (cell.critical) button.classList.add("is-critical");
      button.title = `${row.label} · ${dateTime(new Date(cell.from).toISOString())}–${clock(cell.until, { seconds: true })} · ${peak === undefined ? "観測なし" : `最大 ${peak}% · ${cell.count}観測`}`;
      button.setAttribute("aria-label", button.title);
      return button;
    });
    buttons.forEach((button) =>
      button.addEventListener("focus", () => {
        buttons.forEach((item) => {
          item.tabIndex = item === button ? 0 : -1;
        });
      }),
    );
    buttons.forEach((button, index) =>
      button.addEventListener("keydown", (event) => {
        if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key))
          return;
        event.preventDefault();
        const next =
          event.key === "Home"
            ? 0
            : event.key === "End"
              ? buttons.length - 1
              : Math.max(
                  0,
                  Math.min(
                    buttons.length - 1,
                    index + (event.key === "ArrowLeft" ? -1 : 1),
                  ),
                );
        button.tabIndex = -1;
        const target = buttons[next];
        if (target) {
          target.tabIndex = 0;
          target.focus();
        }
      }),
    );
    cells.append(...buttons);
    root.append(
      el("div", "heatmap-row", el("span", "heatmap-label", row.label), cells),
    );
  }
  root.append(
    el(
      "div",
      "heatmap-key",
      el("span", "", "区間最大 · 0%"),
      el("i"),
      el("span", "", "100% · 斜線＝欠測"),
    ),
  );
  return root;
}
