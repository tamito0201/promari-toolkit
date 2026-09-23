/**
 * Domain vocabulary shared conceptually with the PHP domain. DOM-independent.
 */

/**
 * Click actions matching the PHP Action enum.
 */
export const Action = { Open: 'open', Copy: 'copy', Native: 'native' } as const;
export type Action = (typeof Action)[keyof typeof Action];

/**
 * Placements matching the PHP Placement enum.
 */
export type Placement = 'article_top' | 'article_bottom' | 'sidebar' | 'floating' | 'inline';

/**
 * Share request fields available to URL templates.
 */
export type RequestField = 'url' | 'title' | 'text' | 'via' | 'site' | 'hashtagsCsv';

/**
 * Service specifications extracted from PHP into the generated catalog.
 */
export interface ServiceSpec {
  readonly key: string;
  readonly action: Action;
  readonly endpoint: string;
  readonly params: Readonly<Record<string, RequestField>>;
}

/** 共有先の選択に必要な値だけを表す。描画設定は含めない。 */
export interface SelectionConfig {
  readonly services: readonly string[];
  readonly secondary: readonly string[];
  readonly buttons: Readonly<Record<string, { readonly floating?: boolean }>>;
  readonly floating: { readonly services: readonly string[]; readonly secondaryMax: number };
}

export interface UtmConfig {
  readonly enabled: boolean;
  readonly source: string;
  readonly medium: string;
  readonly campaign: string;
  readonly content: string;
}
