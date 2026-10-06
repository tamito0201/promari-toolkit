// 履歴を時系列に読み替える純粋関数。記録された時刻・値だけを使い、欠測や範囲外を補完しない。
import type { CodexPoint } from "./model.ts";

export interface Point {
  readonly at: number;
  readonly value: number;
}
export interface Series {
  readonly name: string;
  readonly points: readonly Point[];
}
export interface CodexReading {
  readonly name: string;
  readonly usedPct: number;
}
interface WindowObservation {
  readonly minutes: number;
  readonly at: number;
  readonly value: number;
}

const MINUTES_PER_HOUR = 60;
const MINUTES_PER_DAY = 24 * MINUTES_PER_HOUR;

/** windowName は時間枠の長さを短く書く: 10080 → "7d"、300 → "5h"、30 → "30m"。 */
export function windowName(minutes: number): string {
  if (minutes % MINUTES_PER_DAY === 0) return `${minutes / MINUTES_PER_DAY}d`;
  if (minutes % MINUTES_PER_HOUR === 0)
    return `${minutes / MINUTES_PER_HOUR}h`;
  return `${minutes}m`;
}

function validWindow(
  at: number,
  rateWindow: CodexPoint["primary"],
): WindowObservation | undefined {
  if (
    rateWindow === undefined ||
    !Number.isFinite(at) ||
    !Number.isFinite(rateWindow.usedPct) ||
    !Number.isFinite(rateWindow.windowMinutes) ||
    rateWindow.windowMinutes <= 0
  )
    return undefined;
  return { minutes: rateWindow.windowMinutes, at, value: rateWindow.usedPct };
}

/** Codex の primary / secondary の順に依存せず、実際の時間枠ごとに系列化する。 */
export function codexSeries(history: readonly CodexPoint[]): readonly Series[] {
  const observations = history.flatMap((point) => {
    const at = Date.parse(point.at);
    return [point.primary, point.secondary].flatMap(
      (rateWindow) => validWindow(at, rateWindow) ?? [],
    );
  });
  const byWindow = Map.groupBy(observations, (item) => item.minutes);
  return [...byWindow]
    .toSorted(([a], [b]) => a - b)
    .map(([minutes, items]) => ({
      name: `Codex · ${windowName(minutes)}`,
      // 同じ時刻の記録は後のものを採り、最初に現れた位置を保つ。
      points: [...new Map(items.map((item) => [item.at, item.value]))].map(
        ([at, value]) => ({ at, value }),
      ),
    }));
}

/**
 * latestCodex はステータスラインが先頭に描く primary 時間枠の、最新の記録を返す。
 * 記録が無ければ undefined（ゼロで補完しない）。
 */
export function latestCodex(
  history: readonly CodexPoint[],
): CodexReading | undefined {
  const latest = history
    .flatMap((point) => validWindow(Date.parse(point.at), point.primary) ?? [])
    .toSorted((a, b) => a.at - b.at)
    .at(-1);
  return latest === undefined
    ? undefined
    : { name: windowName(latest.minutes), usedPct: latest.value };
}

export function historyPoints(
  points: readonly Point[],
  from: number,
  until: number,
): readonly Point[] {
  return points
    .filter(
      (p) =>
        Number.isFinite(p.at) &&
        Number.isFinite(p.value) &&
        p.at >= from &&
        p.at <= until,
    )
    .toSorted((a, b) => a.at - b.at);
}
