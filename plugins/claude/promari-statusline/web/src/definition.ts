// 指標定義の語彙。数値の意味（単位）と一次情報の出典をここで閉じる。

/** 値の単位。表示の書式と目盛りの扱いはこの語彙だけで決める。 */
export type Unit =
  | "usd"
  | "percent"
  | "ratio"
  | "seconds"
  | "bytes"
  | "days"
  | "decimal"
  | "number";

/** 出典は固定カタログの HTTPS リンクだけ。ログ由来の文字列を URL にしない。 */
export interface Source {
  readonly label: string;
  readonly href: `https://${string}`;
}

export const SOURCES = {
  tail: {
    label: "Dean & Barroso (2013), The Tail at Scale",
    href: "https://research.google/pubs/the-tail-at-scale/",
  },
  monitoring: {
    label: "Anthropic, Claude Code monitoring",
    href: "https://code.claude.com/docs/en/monitoring-usage",
  },
  wilson: {
    label: "Wilson (1927), Probable Inference",
    href: "https://doi.org/10.1080/01621459.1927.10502953",
  },
  shannon: {
    label: "Shannon (1948), A Mathematical Theory of Communication",
    href: "https://doi.org/10.1002/j.1538-7305.1948.tb01338.x",
  },
  hill: {
    label: "Hill (1973), Diversity and Evenness",
    href: "https://doi.org/10.2307/1934352",
  },
  agents: {
    label: "Kapoor et al. (2024), AI Agents That Matter",
    href: "https://arxiv.org/abs/2407.01502",
  },
  swe: {
    label: "Yang et al. (2024), SWE-agent",
    href: "https://arxiv.org/abs/2405.15793",
  },
} as const satisfies Record<string, Source>;
export type SourceId = keyof typeof SOURCES;

/**
 * 指標1件の定義。assessment が "neutral" の指標は高低に優劣を付けず、
 * 評価色（良い・注意・危険）を使わない。
 */
export interface MeasurementDefinition {
  readonly label: string;
  readonly group: string;
  readonly unit: Unit;
  readonly description: string;
  readonly source?: SourceId;
  readonly assessment?: "neutral";
}
