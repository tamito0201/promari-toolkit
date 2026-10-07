import type { Snapshot, Tone } from "./model.ts";
import {
  MEASUREMENTS,
  measurementSource,
  measurementNumber,
  measurementText,
  type MeasurementKey,
} from "./measurements.ts";

const PERCENT = 100;
const BATTERY_WARNING_PERCENT = 20;
const TOOL_ERROR_WARNING_PERCENT = 2;
const TOOL_ERROR_CRITICAL_PERCENT = 5;
const LIMIT_CRITICAL_PERCENT = 90;
const GIB = 2 ** 30;
const UNITS = {
  usd: "$",
  percent: "%",
  ratio: "×",
  seconds: "s",
  bytes: "GiB",
  days: "d",
  decimal: "",
  number: "",
} as const;

// 目盛り・基準・欠測も表示契約に含める。View は数値の意味や分母を推測しない。
export interface InstrumentVM {
  readonly key: string;
  readonly label: string;
  readonly text: string;
  readonly value: number | undefined;
  readonly maximum: number;
  readonly maximumText: string;
  readonly fraction: number | undefined;
  readonly reference:
    | { readonly fraction: number; readonly text: string }
    | undefined;
  readonly basis: string;
  readonly tone: Tone;
  readonly unit: string;
  readonly percentage: boolean;
  readonly href: string | undefined;
}
export interface VisualVM {
  readonly kind: "visual";
  readonly layout:
    | "bars"
    | "rings"
    | "composition"
    | "balance"
    | "columns"
    | "waffle"
    | "lollipop"
    | "time"
    | "range";
  readonly title: string;
  readonly note: string;
  readonly items: readonly InstrumentVM[];
  readonly total: string | undefined;
}

// 自動目盛りは上限や目標ではない。ゼロだけの集合にもゼロの位置を確保する。
export function niceMaximum(value: number): number {
  if (!Number.isFinite(value) || value <= 0) return 1;
  const power = Math.max(Number.MIN_VALUE, 10 ** Math.floor(Math.log10(value)));
  const scaled = value / power;
  const step = scaled <= 1 ? 1 : scaled <= 2 ? 2 : scaled <= 5 ? 5 : 10;
  return Math.min(Number.MAX_VALUE, step * power);
}
export function axisNumber(value: number): string {
  if (value !== 0 && Math.abs(value) < 0.01) return value.toPrecision(2);
  return Intl.NumberFormat("en-US", {
    notation: "compact",
    maximumFractionDigits: 2,
  }).format(value);
}
const REFERENCE_KEYS: Partial<Record<MeasurementKey, MeasurementKey>> = {
  todoDone: "todoTotal",
  staged: "changed",
  conventional: "commitsToday",
  turnP50: "turnP90",
  cpuLoad: "cores",
  deep: "active",
  longest: "active",
};
function metricTone(key: MeasurementKey, value: number | undefined): Tone {
  if (value === undefined) return "muted";
  if (measurementSource(key)) return "info";
  if (key === "battery")
    return value < BATTERY_WARNING_PERCENT ? "danger" : "good";
  if (key === "toolErrors")
    return value >= TOOL_ERROR_CRITICAL_PERCENT
      ? "danger"
      : value >= TOOL_ERROR_WARNING_PERCENT
        ? "caution"
        : "good";
  if (key === "rules" || key === "untested" || key === "intervention")
    return value > 0 ? "caution" : "good";
  const unit = MEASUREMENTS[key].unit;
  return unit === "usd" ? "money" : unit === "percent" ? "good" : "accent";
}
export function instrument(s: Snapshot, key: MeasurementKey): InstrumentVM {
  const { label, unit } = MEASUREMENTS[key];
  const raw = measurementNumber(s, key);
  const value =
    raw === undefined ? undefined : unit === "bytes" ? raw / GIB : raw;
  const referenceKey = REFERENCE_KEYS[key];
  const referenceValue = referenceKey
    ? measurementNumber(s, referenceKey)
    : undefined;
  const maximum =
    unit === "percent"
      ? Math.max(PERCENT, niceMaximum(value ?? 0))
      : niceMaximum(Math.max(value ?? 0, referenceValue ?? 0));
  const base =
    unit === "percent" ? "100% 基準" : "自動目盛 · 上限値ではありません";
  return {
    key,
    label,
    text: measurementText(s, key),
    value,
    maximum,
    maximumText: `${axisNumber(maximum)}${UNITS[unit]}`,
    fraction:
      value === undefined
        ? undefined
        : Math.max(0, Math.min(1, value / maximum)),
    reference:
      referenceKey && referenceValue !== undefined
        ? {
            fraction: Math.max(0, Math.min(1, referenceValue / maximum)),
            text: `${MEASUREMENTS[referenceKey].label} ${measurementText(s, referenceKey)}`,
          }
        : undefined,
    basis: key === "cpuLoad" ? "Load average · CPU数は参考線" : base,
    tone: metricTone(key, raw),
    unit: UNITS[unit],
    percentage: unit === "percent",
    href: `#metric/${key}`,
  };
}
export function percentageInstrument(
  key: string,
  label: string,
  value: number | undefined,
  href?: string,
): InstrumentVM {
  const valid =
    value !== undefined && Number.isFinite(value) ? value : undefined;
  const maximum = Math.max(PERCENT, niceMaximum(valid ?? 0));
  return {
    key,
    label,
    text: valid === undefined ? "未取得" : `${Math.round(valid)}%`,
    value: valid,
    maximum,
    maximumText: `${maximum}%`,
    fraction:
      valid === undefined
        ? undefined
        : Math.max(0, Math.min(1, valid / maximum)),
    reference: undefined,
    basis: "100% 基準",
    tone:
      valid === undefined
        ? "muted"
        : valid >= LIMIT_CRITICAL_PERCENT
          ? "danger"
          : "accent",
    unit: "%",
    percentage: true,
    href,
  };
}

// 比較する対象は呼び出し側が選ぶ。期間の違う費用や違う単位を合計しない。
export function compareMetrics(
  s: Snapshot,
  keys: readonly MeasurementKey[],
  title: string,
  note: string,
  sharedScale = true,
): VisualVM {
  const items = keys.map((key) => instrument(s, key));
  const units = new Set(keys.map((key) => MEASUREMENTS[key].unit));
  const common = sharedScale && units.size === 1;
  const maximum = Math.max(
    units.has("percent") ? PERCENT : 0,
    niceMaximum(Math.max(0, ...items.map((item) => item.value ?? 0))),
  );
  return {
    kind: "visual",
    layout: "bars",
    title,
    note,
    total: undefined,
    items: common
      ? items.map((item) => ({
          ...item,
          maximum,
          maximumText: `${axisNumber(maximum)}${item.unit}`,
          fraction:
            item.value === undefined
              ? undefined
              : Math.max(0, Math.min(1, item.value / maximum)),
          reference: undefined,
          basis: `共通目盛 0–${axisNumber(maximum)}${item.unit}`,
        }))
      : items,
  };
}
export function rings(
  s: Snapshot,
  keys: readonly MeasurementKey[],
  title: string,
): VisualVM {
  return {
    kind: "visual",
    layout: "rings",
    title,
    note: "",
    items: keys.map((key) => instrument(s, key)),
    total: undefined,
  };
}
export function tokenComposition(s: Snapshot): VisualVM {
  const items = [instrument(s, "inputTokens"), instrument(s, "outputTokens")];
  const complete = items.every(
    (item) => item.value !== undefined && item.value >= 0,
  );
  const total = complete
    ? items.reduce((sum, item) => sum + item.value!, 0)
    : undefined;
  return {
    kind: "visual",
    layout: "composition",
    title: "TOKEN FLOW",
    note: "入力にはキャッシュ読み書きを含む",
    total: total === undefined ? undefined : axisNumber(total),
    items: items.map((item) => ({
      ...item,
      fraction:
        total === undefined || total === 0 ? undefined : item.value! / total,
      basis:
        total === 0
          ? "入出力ともに0"
          : total === undefined
            ? "—"
            : "入出力合計に対する構成比",
    })),
  };
}
/** 任意の組の構成比。全員の値が観測できたときだけ合計を1として割る。 */
export function compositionMetrics(
  s: Snapshot,
  keys: readonly MeasurementKey[],
  title: string,
  note: string,
): VisualVM {
  const items = keys.map((key) => instrument(s, key));
  const complete = items.every(
    (item) => item.value !== undefined && item.value >= 0,
  );
  const total = complete
    ? items.reduce((sum, item) => sum + item.value!, 0)
    : undefined;
  return {
    kind: "visual",
    layout: "composition",
    title,
    note,
    total: total === undefined ? undefined : axisNumber(total),
    items: items.map((item) => ({
      ...item,
      fraction:
        total === undefined || total === 0 ? undefined : item.value! / total,
      basis:
        total === 0
          ? "全部が0"
          : total === undefined
            ? "—"
            : "合計に対する構成比",
    })),
  };
}

export function gitBalance(s: Snapshot): VisualVM {
  const comparison = compareMetrics(
    s,
    ["inserted", "deleted"],
    "CODE CHANGES",
    "HEADとの差分 · 同じ目盛で比較",
  );
  return { ...comparison, layout: "balance" };
}

export function activityColumns(s: Snapshot): VisualVM {
  return {
    ...compareMetrics(
      s,
      ["turns", "tools", "edited", "hooks"],
      "Activity",
      "実行数・編集ファイル数を共通目盛りで比較",
    ),
    layout: "columns",
  };
}
export function qualityTiles(s: Snapshot): VisualVM {
  return {
    ...compareMetrics(
      s,
      ["tests", "builds", "rules", "untested"],
      "Quality",
      "各セルは共通目盛りの1/10 · 値の合計ではありません",
    ),
    layout: "waffle",
  };
}
export function timeAllocation(s: Snapshot): VisualVM {
  const items = [instrument(s, "active"), instrument(s, "idle")];
  const complete = items.every(
    (item) => item.value !== undefined && item.value >= 0,
  );
  const total = complete
    ? items.reduce((sum, item) => sum + item.value!, 0)
    : undefined;
  return {
    kind: "visual",
    layout: "time",
    title: "Observed time",
    note: "保存済みの活動・休止時間の構成比",
    total: total === undefined ? undefined : `${axisNumber(total)}s`,
    items: items.map((item) => ({
      ...item,
      fraction:
        total === undefined || total === 0 ? undefined : item.value! / total,
      basis: "観測済み時間に占める割合",
    })),
  };
}
export interface HeatmapVM {
  readonly rows: readonly {
    readonly label: string;
    readonly cells: readonly {
      readonly from: number;
      readonly until: number;
      readonly count: number;
      readonly peak: number | undefined;
      readonly critical: boolean;
    }[];
  }[];
  readonly from: number;
  readonly until: number;
}
const HEATMAP_BINS = 24;
// 濃淡は各区間の実測ピーク。観測のない区間を0%で埋めない。
export function rateHeatmap(
  series: readonly {
    readonly name: string;
    readonly hidden?: boolean;
    readonly points: readonly { readonly at: number; readonly value: number }[];
  }[],
  from: number,
  until: number,
): HeatmapVM {
  const duration = (until - from) / HEATMAP_BINS;
  return {
    from,
    until,
    rows: series
      .filter((item) => !item.hidden)
      .map((series) => ({
        label: series.name,
        cells: Array.from({ length: HEATMAP_BINS }, (_, index) => {
          const start = from + index * duration,
            end = from + (index + 1) * duration;
          const points =
            duration > 0
              ? series.points.filter(
                  (point) =>
                    Number.isFinite(point.value) &&
                    point.at >= start &&
                    (index === HEATMAP_BINS - 1
                      ? point.at <= end
                      : point.at < end),
                )
              : [];
          return {
            from: start,
            until: end,
            count: points.length,
            critical: points.some(
              (point) => point.value >= LIMIT_CRITICAL_PERCENT,
            ),
            peak: points.length
              ? Math.max(...points.map((point) => point.value))
              : undefined,
          };
        }),
      })),
  };
}
