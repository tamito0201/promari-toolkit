// 数値・時刻を表示文へ書く純粋関数。DOM を持たないので node --test で確かめられる。
import type { Tone } from "./model.ts";

const DECIMAL = {
  1: new Intl.NumberFormat("en-US", { maximumFractionDigits: 1 }),
  2: new Intl.NumberFormat("en-US", { maximumFractionDigits: 2 }),
  3: new Intl.NumberFormat("en-US", { maximumFractionDigits: 3 }),
} as const;
const COMPACT = {
  1: new Intl.NumberFormat("en-US", {
    notation: "compact",
    maximumFractionDigits: 1,
  }),
  2: new Intl.NumberFormat("en-US", {
    notation: "compact",
    maximumFractionDigits: 2,
  }),
} as const;
const GROUPED = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 });
const MINUTE_MS = 60_000;
const MINUTES_PER_HOUR = 60;
const HOURS_PER_DAY = 24;

/** decimal は小数点以下を最大 digits 桁で書く: 1.25 → "1.3"（digits=1）。 */
export function decimal(value: number, digits: keyof typeof DECIMAL): string {
  return DECIMAL[digits].format(value);
}

/** compact は大きな数を短く書く: 4_810_000 → "4.8M"。 */
export function compact(value: number, digits: keyof typeof COMPACT): string {
  return COMPACT[digits].format(value);
}

/** severity is the tone of a usage percentage, with the status line's marks. */
export function severity(pct: number, warn = 50, bad = 80): Tone {
  if (pct >= bad) return "danger";
  if (pct >= warn) return "caution";
  return "good";
}

/** usd writes dollars as the status line does: two decimals below $1,000. */
export function usd(v: number): string {
  if (v >= 1000) return `$${GROUPED.format(Math.round(v))}`;
  return `$${v.toFixed(2)}`;
}

/** duration writes a span of time in the status line's short form: 4h04m, 2d20h, 59m. */
export function duration(ms: number): string {
  const minutes = Math.max(0, Math.floor(ms / MINUTE_MS));
  if (minutes < MINUTES_PER_HOUR) return `${minutes}m`;
  const hours = Math.floor(minutes / MINUTES_PER_HOUR);
  if (hours < HOURS_PER_DAY)
    return `${hours}h${pad2(minutes % MINUTES_PER_HOUR)}m`;
  return `${Math.floor(hours / HOURS_PER_DAY)}d${hours % HOURS_PER_DAY}h`;
}

/** pad2 writes a number of two digits: 01 … 06. */
export function pad2(n: number): string {
  return String(n).padStart(2, "0");
}

/** clock は時刻を hh:mm（seconds 指定で hh:mm:ss）で書く。 */
export function clock(
  at: string | number,
  { seconds = false }: { readonly seconds?: boolean } = {},
): string {
  const date = new Date(at);
  const time = `${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
  return seconds ? `${time}:${pad2(date.getSeconds())}` : time;
}

/** dateTime は記録時刻（ISO 文字列またはエポックミリ秒）を日本語の日時で書く。 */
export function dateTime(at: string | number): string {
  return new Date(at).toLocaleString("ja-JP", { hour12: false });
}
