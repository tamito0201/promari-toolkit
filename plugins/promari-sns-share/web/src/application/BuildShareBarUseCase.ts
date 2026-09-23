/**
 * Use case: build a button view model from configuration and page context without accessing the DOM.
 */
import type { LabelStyle, ShareConfig } from './ShareConfig.ts';
import type { DisplayCatalog, DisplayService } from './DisplayCatalog.ts';
export type { Placement } from '../domain/model/ShareTypes.ts';
import { ShareRequest } from '../domain/model/ShareRequest.ts';
import { Action, type Placement } from '../domain/model/ShareTypes.ts';
import { ClickPolicy } from '../domain/service/ClickPolicy.ts';
import { ServiceSelectionPolicy } from '../domain/service/ServiceSelectionPolicy.ts';
import { ShareTextPolicy } from '../domain/service/ShareTextPolicy.ts';
import { UtmPolicy } from '../domain/service/UtmPolicy.ts';

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

export class BuildShareBarUseCase {
  readonly #catalog: DisplayCatalog;

  constructor(catalog: DisplayCatalog) {
    this.#catalog = catalog;
  }

  execute(config: ShareConfig, input: BuildInput): ShareBarViewModel {
    const { url, title, site, placement, canNativeShare } = input;
    const request = ShareRequest.create({
      url,
      title,
      text: ShareTextPolicy.fill(config.text.title_template, { url, title, site }),
      hashtags: config.text.hashtags,
      via: config.text.via,
      site,
    });
    const keys = ServiceSelectionPolicy.select(config, { placement, canNativeShare, repository: this.#catalog.repository });
    return Object.freeze({
      placement,
      heading: placement === 'floating' || config.appearance.heading_position === 'none' ? '' : config.heading,
      headingPosition: config.appearance.heading_position,
      groupLabel: config.messages.group_label,
      primary: this.#catalog.resolve(keys.primary).map((service) => this.#button(config, request, 'primary', service)),
      secondary: this.#catalog.resolve(keys.secondary).map((service) => this.#button(config, request, 'secondary', service)),
    });
  }

  #button(config: ShareConfig, request: ShareRequest, tier: Tier, service: DisplayService): ButtonViewModel {
    const { key } = service;
    const label = config.labels[key] ?? service.appearance.label;
    const href = service.shareUrl(request.withUrl(UtmPolicy.apply(request.url, config.utm, key)));
    const isOpen = service.action === Action.Open;
    const popup = isOpen && config.behavior.popup && ClickPolicy.canOpenInPopup(href);
    const override = <T extends string | boolean>(name: 'color' | 'label_style' | 'tooltip', fallback: T): T => {
      const value = config.buttons[key]?.[name];
      return value === undefined || value === '' ? fallback : (value as T);
    };
    return Object.freeze({
      key,
      tier,
      href,
      label,
      tooltip: override('tooltip', label),
      color: override('color', service.appearance.color),
      icon: service.appearance.icon,
      action: service.action,
      labelStyle: tier === 'primary' ? override<LabelStyle>('label_style', config.appearance.label_style) : 'icon',
      newTab: isOpen && config.behavior.open_in_new_tab,
      nofollow: isOpen && config.behavior.nofollow,
      popup: popup ? { width: config.behavior.popup_width, height: config.behavior.popup_height } : null,
      url: request.url,
      title: request.title,
    });
  }
}
