// API のデータ契約。I/O や画面の状態を持たず、取得値は読み取り専用で扱う。
// 省略可能な項目は Go 側が省略する（omitzero）ものに限る。検証は snapshot.ts が境界で行う。
export const TONES = [
  "",
  "accent",
  "good",
  "caution",
  "danger",
  "info",
  "money",
  "note",
  "muted",
  "brand",
] as const;
export type Tone = (typeof TONES)[number];

export interface Span {
  readonly text: string;
  readonly tone?: Tone | undefined;
  readonly bold?: boolean | undefined;
  readonly alarm?: boolean | undefined;
}

export interface Chip {
  readonly spans: readonly Span[];
}

export interface Group {
  readonly band: number;
  readonly title: string;
  readonly tone: Tone;
  readonly chips: readonly Chip[];
}

export interface Band {
  readonly band: number;
  readonly name: string;
  readonly question: string;
}

export interface RateWindow {
  readonly usedPct: number;
  readonly resetsAt?: string | undefined;
}

export interface Headline {
  readonly contextPct?: number | undefined;
  readonly fiveHour?: RateWindow | undefined;
  readonly sevenDay?: RateWindow | undefined;
  readonly sessionUsd?: number | undefined;
  readonly todayUsd?: number | undefined;
  readonly running: number;
}

export interface CodexWindow {
  readonly usedPct: number;
  readonly windowMinutes: number;
}

export interface CodexPoint {
  readonly at: string;
  readonly primary?: CodexWindow | undefined;
  readonly secondary?: CodexWindow | undefined;
}

export interface RatePoint {
  readonly at: string;
  readonly fiveHour?: number | undefined;
  readonly sevenDay?: number | undefined;
}

export interface ContextPoint {
  readonly at: string;
  readonly tokens: number;
}

export interface Measurement {
  readonly value?: number | undefined;
  readonly note?: string | undefined;
}

export interface Session {
  readonly name?: string | undefined;
  readonly model?: string | undefined;
  readonly project?: string | undefined;
  readonly version?: string | undefined;
}

export interface Snapshot {
  readonly version: string;
  readonly at: string;
  readonly inputAt?: string | undefined;
  readonly live: boolean;
  readonly limitsAt?: string | undefined;
  readonly session: Session;
  readonly headline: Headline;
  readonly bands: readonly Band[];
  readonly groups: readonly Group[];
  readonly rateHistory: readonly RatePoint[];
  readonly codexHistory: readonly CodexPoint[];
  readonly contextHistory: readonly ContextPoint[];
  readonly measurements: Readonly<Record<string, Measurement>>;
}
