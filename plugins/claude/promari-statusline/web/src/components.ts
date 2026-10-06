import type {
  MetricVM,
  StatVM,
  PanelVM,
  BlockVM,
  CardVM,
} from "./presentation.ts";
import { el, link } from "./dom.ts";
import { renderVisual, renderGauge } from "./visualization-view.ts";
import { renderChart } from "./chart-view.ts";

// すべての部品は表示用の値を受け取るだけで、API やアプリケーション状態を参照しない。
export function renderMetric(data: MetricVM): HTMLElement {
  const node = el(
    "div",
    `metric${data.known ? "" : " unavailable"}`,
    el("span", "metric-label", data.label),
    el(
      "strong",
      `metric-value${data.value.length > 9 ? " compact" : ""}${data.tone ? ` tone-${data.tone}` : ""}`,
      data.value,
    ),
  );
  node.dataset["known"] = String(data.known);
  if (data.key) node.dataset["measurement"] = data.key;
  node.title = `${data.label}: ${data.value}${data.explanation ? ` · ${data.explanation}` : ""}`;
  return node;
}
function metrics(items: readonly MetricVM[], two = false): HTMLElement {
  return el(
    "div",
    `metric-grid${two ? " two" : ""}`,
    ...items.map(renderMetric),
  );
}
export function renderStat(data: StatVM): HTMLElement {
  const { label, value, note, ratio, featured } = data;
  const node = link(
    "",
    data.href,
    `stat${ratio !== undefined && ratio >= 90 ? " is-critical" : ""}${featured ? " featured" : ""}`,
  );
  node.title = `${label}: ${value} · ${note}`;
  node.append(
    el("div", "stat-label", label, el("span", "", "↗")),
    el("strong", "stat-value", value),
    el("p", "stat-note", note),
  );
  if (data.gauge) {
    node.classList.add("has-gauge");
    node.append(renderGauge(data.gauge, true));
  }
  if (ratio !== undefined) {
    const bar = el("div", "stat-bar", el("i"));
    bar.style.setProperty("--value", `${Math.max(0, Math.min(100, ratio))}%`);
    node.append(bar);
  }
  return node;
}
function renderAlerts(
  data: Extract<BlockVM, { kind: "alerts" }>,
): HTMLElement[] {
  const summary = el(
    "div",
    "alert-summary",
    el("strong", "alert-number", String(data.count)),
    el("span", "alert-caption", "警告のある分類"),
  );
  const list = el("div", "alert-list");
  for (const item of data.items) {
    const a = link("", item.href, "alert-item");
    a.append(el("b", "", item.label), el("span", "", item.text));
    a.title = `${item.label}: ${item.text}`;
    list.append(a);
  }
  if (!data.count)
    list.append(el("p", "alert-caption", "取得した指標に警告はありません。"));
  const monitors = el(
    "div",
    "monitor-grid",
    ...data.monitors.map((item) => {
      const a = link(
        "",
        item.href,
        `monitor-cell${item.alarm ? " alert" : item.stale ? " stale" : ""}`,
      );
      a.append(
        el("span", "monitor-icon", item.alarm ? "!" : item.stale ? "◷" : "✓"),
        el("span", "", item.label),
      );
      a.title = `${item.label}: ${item.state}`;
      a.setAttribute("aria-label", a.title);
      return a;
    }),
  );
  return [summary, list, monitors];
}
function renderBlock(
  data: BlockVM,
  detail: boolean,
  width: number,
): HTMLElement[] {
  switch (data.kind) {
    case "metrics":
      return [metrics(data.items, data.two)];
    case "text": {
      const node = el("p", data.style, data.text);
      node.title = data.text;
      return [node];
    }
    case "visual":
      return [renderVisual(data)];
    case "alerts":
      return renderAlerts(data);
    case "chart":
      return renderChart(data, detail, width);
    default:
      return unreachable(data);
  }
}
function unreachable(value: never): never {
  throw new Error(`未対応の表示部品: ${String(value)}`);
}
function previewButton(route: string, title: string): HTMLButtonElement {
  const button = el("button", "panel-expand", "⤢");
  button.type = "button";
  button.dataset["preview"] = route;
  button.setAttribute("aria-label", `${title}を拡大`);
  button.setAttribute("aria-controls", "focus-preview");
  button.setAttribute("aria-expanded", "false");
  button.title = "重ねる・フォーカスで拡大。クリックで拡大図を操作";
  return button;
}
export function renderPanel(
  data: PanelVM,
  width: number,
  detail = false,
): HTMLElement {
  const node = el(
    "article",
    `panel ${data.kind}${data.route ? " is-linked" : ""}`,
    el(
      "header",
      "panel-head",
      el("span", "panel-number", data.number),
      el(
        "h2",
        "",
        data.route ? link(data.title, `#detail/${data.route}`) : data.title,
      ),
      el("span", "panel-meta", data.meta),
      data.route ? previewButton(data.route, data.title) : null,
    ),
    el(
      "div",
      "panel-body",
      ...data.blocks.flatMap((block) => renderBlock(block, detail, width)),
    ),
  );
  if (data.route) node.dataset["panel"] = data.route;
  return node;
}
export function renderCard(data: CardVM): HTMLElement {
  const card = el(
    "article",
    `card${data.alarm ? " is-alarm" : ""}`,
    el(
      "header",
      "card-head",
      el("span", "card-icon", "◈"),
      el("h3", "", data.title),
      el("span", "card-state", data.state),
    ),
    el(
      "ul",
      "chips",
      ...data.chips.map((chip) =>
        el(
          "li",
          `chip${chip.alarm ? " is-alarm" : ""}`,
          ...chip.pieces.map((piece) => {
            if (piece.kind === "text") {
              const s = piece.span;
              return el(
                "span",
                `${s.tone ? `tone-${s.tone}` : ""}${s.bold ? " is-bold" : ""}`,
                s.text,
              );
            }
            const fill = el("span", "bar-fill");
            fill.style.setProperty("--fill", String(piece.ratio));
            const bar = el("span", `bar tone-${piece.tone}`, fill);
            bar.setAttribute("role", "img");
            bar.setAttribute("aria-label", `${Math.round(piece.ratio * 100)}%`);
            return bar;
          }),
        ),
      ),
    ),
  );
  card.id = data.id;
  return card;
}

// 出典は固定カタログから来るHTTPSリンク。ログの文字列をURLとして使わない。
export function renderSource(data: MetricVM): HTMLElement | null {
  if (!data.source) return null;
  const node = link(
    `出典: ${data.source.label} ↗`,
    data.source.href,
    "measurement-source",
  );
  node.target = "_blank";
  node.rel = "noopener noreferrer";
  return node;
}
