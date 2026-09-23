/**
 * Use case: build a button view model from configuration and page context without accessing the DOM.
 */
import type { LabelStyle, ShareSettings } from './ShareSettings.ts';
import type { ShareButtonCatalog, DisplayedShareDestination } from './ShareButtonCatalog.ts';
export type { Placement } from '../domain/model/Placement.ts';
import { ShareRequest } from '../domain/model/ShareRequest.ts';
import { ShareAction } from '../domain/model/ShareAction.ts';
import { type Placement } from '../domain/model/Placement.ts';
import { ShareActionPolicy } from '../domain/service/ShareActionPolicy.ts';
import { DestinationSelectionPolicy } from '../domain/service/DestinationSelectionPolicy.ts';
import { ShareTextFormatter } from '../domain/service/ShareTextFormatter.ts';
import { UtmParameterPolicy } from '../domain/service/UtmParameterPolicy.ts';

export type ShareButtonTier = 'primary' | 'secondary';

export interface ShareButtonViewModel {
  readonly key: string;
  readonly tier: ShareButtonTier;
  readonly href: string;
  readonly label: string;
  readonly tooltip: string;
  readonly color: string;
  readonly icon: string;
  readonly action: ShareAction;
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
  readonly headingPosition: ShareSettings['appearance']['heading_position'];
  readonly groupLabel: string;
  readonly primary: readonly ShareButtonViewModel[];
  readonly secondary: readonly ShareButtonViewModel[];
}

export interface BuildShareBarInput {
  readonly url: string;
  readonly title: string;
  readonly site: string;
  readonly placement: Placement;
  readonly canNativeShare: boolean;
}

export class BuildShareBarUseCase {
  readonly #catalog: ShareButtonCatalog;

  constructor(catalog: ShareButtonCatalog) {
    this.#catalog = catalog;
  }

  execute(config: ShareSettings, input: BuildShareBarInput): ShareBarViewModel {
    const { url, title, site, placement, canNativeShare } = input;
    const request = ShareRequest.create({
      url,
      title,
      text: ShareTextFormatter.format(config.text.title_template, { url, title, site }),
      hashtags: config.text.hashtags,
      via: config.text.via,
      site,
    });
    const keys = DestinationSelectionPolicy.select(config, { placement, canNativeShare, repository: this.#catalog.repository });
    return Object.freeze({
      placement,
      heading: placement === 'floating' || config.appearance.heading_position === 'none' ? '' : config.heading,
      headingPosition: config.appearance.heading_position,
      groupLabel: config.messages.group_label,
      primary: this.#catalog.resolve(keys.primary).map((destination) => this.#button(config, request, 'primary', destination)),
      secondary: this.#catalog.resolve(keys.secondary).map((destination) => this.#button(config, request, 'secondary', destination)),
    });
  }

  #button(config: ShareSettings, request: ShareRequest, tier: ShareButtonTier, destination: DisplayedShareDestination): ShareButtonViewModel {
    const { key } = destination;
    const label = config.labels[key] ?? destination.appearance.label;
    const href = destination.shareUrl(request.withUrl(UtmParameterPolicy.apply(request.url, config.utm, key)));
    const isOpen = destination.action === ShareAction.Open;
    const popup = isOpen && config.behavior.popup && ShareActionPolicy.canOpenInPopup(href);
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
      color: override('color', destination.appearance.color),
      icon: destination.appearance.icon,
      action: destination.action,
      labelStyle: tier === 'primary' ? override<LabelStyle>('label_style', config.appearance.label_style) : 'icon',
      newTab: isOpen && config.behavior.open_in_new_tab,
      nofollow: isOpen && config.behavior.nofollow,
      popup: popup ? { width: config.behavior.popup_width, height: config.behavior.popup_height } : null,
      url: request.url,
      title: request.title,
    });
  }
}
