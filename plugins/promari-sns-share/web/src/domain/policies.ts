/**
 * Pure policies for button selection, shared text, and URL parameters. Keep these consistent with PHP formatting and selection rules.
 */
import { Action, type Catalog, type Placement, type SelectionConfig, type UtmConfig } from './types.ts';

export interface PageInfo {
  readonly url: string;
  readonly title: string;
  readonly site: string;
}

/**
 * Expand the title, site, and URL placeholders.
 */
export const fillTemplate = (template: string, { url, title, site }: PageInfo): string =>
  template.replaceAll('{title}', title).replaceAll('{site}', site).replaceAll('{url}', url);

/**
 * Append query parameters while preserving fragments.
 */
export const appendQuery = (url: string, params: Readonly<Record<string, string>>): string => {
  const hash = url.indexOf('#');
  const [base, fragment] = hash >= 0 ? [url.slice(0, hash), url.slice(hash + 1)] : [url, undefined];
  const query = Object.entries(params).map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`).join('&');
  return `${base}${base.includes('?') ? '&' : '?'}${query}${fragment === undefined ? '' : `#${fragment}`}`;
};

/**
 * Add UTM parameters when enabled and expand the service placeholder.
 */
export const utmUrl = (url: string, utm: UtmConfig, service: string): string => {
  if (!utm.enabled) return url;
  const params = Object.fromEntries(
    (['source', 'medium', 'campaign', 'content'] as const)
      .map((k) => [`utm_${k}`, utm[k].replaceAll('{service}', service)] as const)
      .filter(([, v]) => v !== ''),
  );
  return Object.keys(params).length ? appendQuery(url, params) : url;
};

export interface Selection {
  readonly primary: readonly string[];
  readonly secondary: readonly string[];
}

/**
 * Select primary and secondary services for the placement and device capabilities. Native sharing requires navigator.share. Floating bars honor service overrides, exclusions, and the secondary limit.
 */
export const selectServices = (
  { services, secondary, buttons, floating }: SelectionConfig,
  { placement, canNativeShare, catalog }: { placement: Placement; canNativeShare: boolean; catalog: Catalog },
): Selection => {
  const isFloating = placement === 'floating';
  const visible = secondary
    .filter((key) => catalog.has(key))
    .filter((key) => canNativeShare || catalog.resolve([key])[0]?.action !== Action.Native)
    .filter((key) => !isFloating || (buttons[key]?.floating ?? true));
  return {
    primary: isFloating && floating.services.length ? floating.services : services,
    secondary: isFloating ? visible.slice(0, floating.secondaryMax) : visible,
  };
};

export type ClickDecision = 'copy' | 'native' | 'popup' | 'follow';

/**
 * Choose the click action; the application layer executes it through ports.
 */
export const decideClick = ({ action, popup }: { action: Action; popup: unknown }): ClickDecision =>
  action === Action.Copy ? 'copy' : action === Action.Native ? 'native' : popup ? 'popup' : 'follow';
