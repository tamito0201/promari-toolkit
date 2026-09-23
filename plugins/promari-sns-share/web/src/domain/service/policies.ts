/**
 * Domain services: pure policies for button selection, shared text, and URL parameters.
 */
import { encodeComponent } from './encoding.ts';
import type { ShareServiceRepository } from '../repository/ShareServiceRepository.ts';
import { Action, type Placement, type SelectionConfig, type UtmConfig } from '../model/types.ts';

export interface PageInfo {
  readonly url: string;
  readonly title: string;
  readonly site: string;
}

/**
 * Expand the title, site, and URL placeholders in a single pass.
 * 置換した結果は走査し直さない。題名に {site} が含まれても、それは題名のまま残る。
 */
export const fillTemplate = (template: string, { url, title, site }: PageInfo): string => {
  const values: Readonly<Record<string, string>> = { '{title}': title, '{site}': site, '{url}': url };
  return template.replace(/\{(?:title|site|url)\}/g, (placeholder) => values[placeholder] ?? placeholder);
};

/**
 * Append query parameters while preserving fragments.
 */
export const appendQuery = (url: string, params: Readonly<Record<string, string>>): string => {
  const hash = url.indexOf('#');
  const [base, fragment] = hash >= 0 ? [url.slice(0, hash), url.slice(hash + 1)] : [url, undefined];
  const query = Object.entries(params).map(([k, v]) => `${encodeComponent(k)}=${encodeComponent(v)}`).join('&');
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
  { placement, canNativeShare, repository }: { placement: Placement; canNativeShare: boolean; repository: ShareServiceRepository },
): Selection => {
  const isFloating = placement === 'floating';
  const visible = secondary
    .filter((key) => repository.has(key))
    .filter((key) => canNativeShare || repository.resolve([key])[0]?.action !== Action.Native)
    .filter((key) => !isFloating || (buttons[key]?.floating ?? true));
  return {
    primary: isFloating && floating.services.length ? floating.services : services,
    secondary: isFloating ? visible.slice(0, floating.secondaryMax) : visible,
  };
};

/**
 * 小窓で開ける共有先か。mailto: はメールソフトへ渡すので、窓を開いても何も残らない。
 */
export const canOpenInPopup = (href: string): boolean => !href.startsWith('mailto:');

export type ClickDecision = 'copy' | 'native' | 'popup' | 'follow';

/**
 * Choose the click action; the application layer executes it through ports.
 */
export const decideClick = ({ action, popup }: { action: Action; popup: unknown }): ClickDecision =>
  action === Action.Copy ? 'copy' : action === Action.Native ? 'native' : popup ? 'popup' : 'follow';
