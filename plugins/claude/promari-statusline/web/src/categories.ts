// ステータスラインの分類（グループ）をページの分類へ読み替える純粋関数。
import type { Band, Chip, Group, Span, Tone } from "./model.ts";

/** A category as the page shows it: an emoji, a name and the chips. */
export interface Category {
  readonly emoji: string;
  readonly name: string;
  readonly tone: Tone;
  readonly chips: readonly Chip[];
}

/** A band and the categories it holds, in display order. */
export interface Section {
  readonly band: Band;
  readonly categories: readonly Category[];
}

/** A piece of a chip: text, or a gauge made of block characters. */
export type Piece =
  | { readonly kind: "text"; readonly span: Span }
  | { readonly kind: "bar"; readonly ratio: number; readonly tone: Tone };

const BAR = /^[█▓▒░]+$/u;
const BAR_CHARACTERS = /[█░▓▒]/gu;

/** trimStart drops the blank spans that stood between a header and its text. */
function trimStart(spans: readonly Span[]): readonly Span[] {
  const first = spans.findIndex((s) => s.text.trim() !== "");
  return first < 0 ? [] : spans.slice(first);
}

/**
 * category splits a group's title into its emoji and name. A group without a
 * title (⚡ Claude, 🤖 Codex) carries its header as the first span of its first
 * chip; that span becomes the title and leaves the chip.
 */
export function category(g: Group): Category {
  const [first, ...rest] = g.chips;
  const head = g.title === "" ? first?.spans[0] : undefined;
  if (first === undefined || head === undefined) return named(g.title, g.tone, g.chips);
  const spans = trimStart(first.spans.slice(1));
  return named(head.text, head.tone ?? "", spans.length > 0 ? [{ spans }, ...rest] : rest);
}
function named(title: string, tone: Tone, chips: readonly Chip[]): Category {
  const [emoji = "", ...words] = title.trim().split(/\s+/u);
  return { emoji, name: words.join(" "), tone, chips };
}

/** barRatio returns how full a block bar ("███░░") is, or null for other text. */
export function barRatio(text: string): number | null {
  if (!BAR.test(text)) return null;
  const cells = [...text];
  return cells.filter((c) => c === "█").length / cells.length;
}

/**
 * pieces joins the block characters of a chip into gauges. The status line
 * draws a bar as two spans, the filled part ("██", coloured by the usage) and
 * the empty part ("░░░", muted), so the spans of one bar are read as one gauge
 * coloured like its filled part.
 */
export function pieces(spans: readonly Span[]): readonly Piece[] {
  const out: Piece[] = [];
  let run: Span[] = [];
  const flush = (): void => {
    if (run.length === 0) return;
    const text = run.map((s) => s.text).join("");
    const filled = run.find((s) => s.text.includes("█")) ?? run[0];
    out.push({ kind: "bar", ratio: barRatio(text) ?? 0, tone: filled?.tone ?? "" });
    run = [];
  };
  for (const s of spans) {
    if (barRatio(s.text) !== null) {
      run.push(s);
      continue;
    }
    flush();
    out.push({ kind: "text", span: s });
  }
  flush();
  return out;
}

/** sections puts the groups under their bands. Every band is kept, empty or not. */
export function sections(
  bands: readonly Band[],
  groups: readonly Group[],
): readonly Section[] {
  const byBand = Map.groupBy(groups, (g) => g.band);
  return bands.map((band) => ({
    band,
    categories: (byBand.get(band.band) ?? []).map(category),
  }));
}

export function chipAlarm(chip: Chip): boolean {
  return chip.spans.some((s) => s.alarm === true);
}
export function hasAlarm(c: Category): boolean {
  return c.chips.some(chipAlarm);
}
export function chipText(chip: Chip): string {
  return chip.spans.map((s) => s.text).join("");
}
/** categoryText は分類の全チップを表示どおりの文で連ねる（検索対象）。 */
export function categoryText(c: Category): string {
  return c.chips.map(chipText).join(" · ");
}
/** alarmText は警告中のチップだけを、ゲージの記号を除いて連ねる。 */
export function alarmText(c: Category): string {
  return c.chips
    .filter(chipAlarm)
    .map(chipText)
    .join(" / ")
    .replace(BAR_CHARACTERS, "")
    .replace(/\s+/gu, " ");
}

/** alarmed lists the categories with a chip in alarm, from every band. */
export function alarmed(
  all: readonly Section[],
): readonly { readonly band: number; readonly category: Category }[] {
  return all.flatMap((s) =>
    s.categories
      .filter(hasAlarm)
      .map((category) => ({ band: s.band.band, category })),
  );
}

/** anchor is the id of a category's row on the page. */
export function anchor(band: number, c: Category): string {
  return `cat-${band}-${c.name.toLowerCase().replace(/[^a-z0-9]+/gu, "-")}`;
}

/** 同じ分類の表示値だけを使い、未取得をゼロで補完しない。 */
export function signalValue(
  categories: readonly Category[],
  name: string,
  expression: RegExp,
): string | undefined {
  const category = categories.find((c) => c.name === name);
  for (const chip of category?.chips ?? []) {
    const match = chip.spans
      .map((s) => s.text)
      .join("")
      .match(expression);
    if (match?.[1] !== undefined) return match[1];
  }
  return undefined;
}

export function signalText(
  categories: readonly Category[],
  name: string,
): string | undefined {
  const category = categories.find((c) => c.name === name);
  return category?.chips
    .map((c) => c.spans.map((s) => s.text).join(""))
    .join(" · ");
}
