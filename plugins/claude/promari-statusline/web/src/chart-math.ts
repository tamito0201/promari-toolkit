// グラフの実測点とキーボード操作の純粋関数。ViewModel と View の双方が使う。
export interface ObservableSeries<C extends string> {
  readonly name: string;
  readonly color: C;
  readonly hidden?: boolean;
  readonly points: readonly { readonly at: number; readonly value: number }[];
}
export interface Observation<C extends string> {
  readonly at: number;
  readonly value: number;
  readonly name: string;
  readonly color: C;
}

const CURSOR_KEYS: ReadonlySet<string> = new Set([
  "ArrowLeft",
  "ArrowRight",
  "Home",
  "End",
]);
const STEP: ReadonlyMap<string, number> = new Map([
  ["ArrowLeft", -1],
  ["ArrowRight", 1],
]);

// カーソルは実測点だけを選ぶ。欠測や系列間の時刻差を補間しない。
export function observations<C extends string>(
  series: readonly ObservableSeries<C>[],
): readonly Observation<C>[] {
  return series
    .filter((item) => !item.hidden)
    .flatMap((item) =>
      item.points
        .filter(
          (point) => Number.isFinite(point.at) && Number.isFinite(point.value),
        )
        .map((point) => ({
          at: point.at,
          value: point.value,
          name: item.name,
          color: item.color,
        })),
    )
    .toSorted((a, b) => a.at - b.at || a.name.localeCompare(b.name));
}

export function nearestObservation(
  points: readonly { readonly at: number }[],
  at: number,
): number {
  if (!Number.isFinite(at)) return -1;
  return points.reduce(
    (best, point, index, all) =>
      best < 0 ||
      Math.abs(point.at - at) < Math.abs((all[best]?.at ?? point.at) - at)
        ? index
        : best,
    -1,
  );
}

/** isCursorKey はカーソル移動に使うキー（左右・先頭・末尾）かを判定する。 */
export function isCursorKey(key: string): boolean {
  return CURSOR_KEYS.has(key);
}

export function cursorIndex(
  index: number,
  key: string,
  length: number,
): number {
  if (!length) return -1;
  if (key === "Home") return 0;
  if (key === "End") return length - 1;
  return Math.max(0, Math.min(length - 1, index + (STEP.get(key) ?? 0)));
}
