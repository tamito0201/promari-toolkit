/**
 * Build a button view model from configuration and page context without accessing the DOM.
 */
import type { LabelStyle, ShareConfig } from './config.ts';
import type { DisplayCatalog, DisplayService } from './ServiceCatalog.ts';
export type { Placement } from '../domain/types.ts';
import { createShareRequest, withUrl } from '../domain/ShareRequest.ts';
import { Action, type Placement, type ShareRequest } from '../domain/types.ts';
import { canOpenInPopup, fillTemplate, selectServices, utmUrl } from '../domain/policies.ts';

export type Tier = 'primary' | 'secondary';

export interface ButtonViewModel {
  readonly key: string;
  readonly tier: Tier;
  readonly href: string;
  readonly label: string;
  readonly tooltip: string;
  readonly color: string;
  readonly icon: string;
  readonly action: Action;
  readonly labelStyle: LabelStyle;
  readonly newTab: boolean;
  readonly nofollow: boolean;
  readonly popup: { readonly width: number; readonly height: number } | null;
  readonly url: string;
  readonly title: string;
}

export interface ShareBarViewModel {
  readonly placement: Placement;
  readonly heading: string;
  readonly headingPosition: ShareConfig['appearance']['heading_position'];
  readonly groupLabel: string;
  readonly primary: readonly ButtonViewModel[];
  readonly secondary: readonly ButtonViewModel[];
}

export interface BuildInput {
  readonly url: string;
  readonly title: string;
  readonly site: string;
  readonly placement: Placement;
  readonly canNativeShare: boolean;
}

const override = <T extends string | boolean>(config: ShareConfig, key: string, name: 'color' | 'label_style' | 'tooltip', fallback: T): T => {
  const value = config.buttons[key]?.[name];
  return value === undefined || value === '' ? fallback : (value as T);
};

const toButton = (config: ShareConfig, request: ShareRequest, tier: Tier) => (service: DisplayService): ButtonViewModel => {
  const { key } = service;
  const label = config.labels[key] ?? service.appearance.label;
  const href = service.shareUrl(withUrl(request, utmUrl(request.url, config.utm, key)));
  const isOpen = service.action === Action.Open;
  const popup = isOpen && config.behavior.popup && canOpenInPopup(href);
  return Object.freeze({
    key,
    tier,
    href,
    label,
    tooltip: override(config, key, 'tooltip', label),
    color: override(config, key, 'color', service.appearance.color),
    icon: service.appearance.icon,
    action: service.action,
    labelStyle: tier === 'primary' ? override<LabelStyle>(config, key, 'label_style', config.appearance.label_style) : 'icon',
    newTab: isOpen && config.behavior.open_in_new_tab,
    nofollow: isOpen && config.behavior.nofollow,
    popup: popup ? { width: config.behavior.popup_width, height: config.behavior.popup_height } : null,
    url: request.url,
    title: request.title,
  });
};

export const buildShareBar = ({ catalog }: { catalog: DisplayCatalog }, config: ShareConfig, input: BuildInput): ShareBarViewModel => {
  const { url, title, site, placement, canNativeShare } = input;
  const request = createShareRequest({
    url,
    title,
    text: fillTemplate(config.text.title_template, { url, title, site }),
    hashtags: config.text.hashtags,
    via: config.text.via,
    site,
  });
  const keys = selectServices(config, { placement, canNativeShare, catalog });
  return Object.freeze({
    placement,
    heading: placement === 'floating' || config.appearance.heading_position === 'none' ? '' : config.heading,
    headingPosition: config.appearance.heading_position,
    groupLabel: config.messages.group_label,
    primary: catalog.resolve(keys.primary).map(toButton(config, request, 'primary')),
    secondary: catalog.resolve(keys.secondary).map(toButton(config, request, 'secondary')),
  });
};
