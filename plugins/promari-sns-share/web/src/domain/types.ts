/**
 * DOM-independent domain types shared conceptually with the PHP domain.
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
  readonly label: string;
  readonly color: string;
  readonly icon: string;
  readonly action: Action;
  readonly endpoint: string;
  readonly params: Readonly<Record<string, RequestField>>;
}

/**
 * Immutable value object describing the shared content.
 */
export interface ShareRequest {
  readonly url: string;
  readonly title: string;
  readonly text: string;
  readonly hashtags: readonly string[];
  readonly via: string;
  readonly site: string;
  readonly hashtagsCsv: string;
}

/**
 * A service entity that constructs share URLs.
 */
export interface Service {
  readonly key: string;
  readonly label: string;
  readonly color: string;
  readonly icon: string;
  readonly action: Action;
  shareUrl(request: ShareRequest): string;
}

/**
 * Immutable service registry.
 */
export interface Catalog {
  has(key: string): boolean;
  keys(): readonly string[];
  resolve(keys: readonly string[]): readonly Service[];
}

export type LabelStyle = 'icon_text' | 'icon' | 'text';
export type Size = 'small' | 'large';
export type Shape = 'official' | 'pill' | 'rounded' | 'square';
export type HeadingPosition = 'left' | 'top' | 'none';
export type SecondaryStyle = 'mono' | 'brand' | 'outline';

export interface ButtonOverride {
  readonly color?: string;
  readonly label_style?: LabelStyle | '';
  readonly tooltip?: string;
  readonly floating?: boolean;
}

/**
 * Web Component configuration derived from TOML by web_defaults.
 */
export interface ShareConfig {
  readonly services: readonly string[];
  readonly secondary: readonly string[];
  readonly labels: Readonly<Record<string, string>>;
  readonly heading: string;
  readonly buttons: Readonly<Record<string, ButtonOverride>>;
  readonly appearance: {
    readonly size: Size;
    readonly label_style: LabelStyle;
    readonly shape: Shape;
    readonly gap_px: number;
    readonly font_family: string;
    readonly heading_position: HeadingPosition;
    readonly secondary_size_px: number;
    readonly secondary_style: SecondaryStyle;
  };
  readonly text: { readonly title_template: string; readonly hashtags: readonly string[]; readonly via: string };
  readonly utm: { readonly enabled: boolean; readonly source: string; readonly medium: string; readonly campaign: string; readonly content: string };
  readonly behavior: { readonly open_in_new_tab: boolean; readonly popup: boolean; readonly popup_width: number; readonly popup_height: number; readonly nofollow: boolean };
  readonly floating: { readonly position: 'bottom' | 'top'; readonly after: number; readonly secondaryMax: number; readonly hideNearEnd: boolean; readonly services: readonly string[] };
  readonly tracking: { readonly attribute: string; readonly event_name: string };
  readonly messages: { readonly copied: string; readonly group_label: string };
  readonly style: { readonly accent: string; readonly floating_background: string };
}

/**
 * Recursive partial configuration for attribute and JSON overrides.
 */
export type DeepPartial<T> = { [K in keyof T]?: T[K] extends readonly (infer _)[] ? T[K] : T[K] extends object ? DeepPartial<T[K]> : T[K] };
