/**
 * The promari-sns-share custom element. Read attributes, call use cases, render into Shadow DOM, and forward clicks. Policies belong to domain; browser effects go through ports.
 */
import { buildShareBar, type ButtonViewModel } from '../application/BuildShareBar.ts';
import { handleShareClick } from '../application/HandleShareClick.ts';
import type { Catalog, Placement, ShareConfig } from '../domain/types.ts';
import { OBSERVED_ATTRIBUTES, readConfig } from '../infrastructure/AttributeConfig.ts';
import { browserClipboard, customEventTracker, pageContext, popupWindow, scrollWatcher, toastNotifier, webShare } from '../infrastructure/browserPorts.ts';
import { buildCss } from './styles.ts';
import { renderHtml } from './view.ts';

const PLACEMENTS: ReadonlySet<string> = new Set<Placement>(['article_top', 'article_bottom', 'sidebar', 'floating', 'inline']);
const asPlacement = (value: string | null): Placement => (value && PLACEMENTS.has(value) ? (value as Placement) : 'inline');

export interface ElementDeps {
  readonly catalog: Catalog;
  readonly defaults: ShareConfig;
}

export const defineElement = ({ catalog, defaults }: ElementDeps): void => {
  class PromariSnsShareElement extends HTMLElement {
    static get observedAttributes(): readonly string[] { return OBSERVED_ATTRIBUTES; }

    #buttons = new Map<string, ButtonViewModel>();
    #stopWatching: (() => void) | null = null;

    connectedCallback(): void {
      this.#render();
      if (asPlacement(this.getAttribute('placement')) === 'floating') {
        const { floating } = readConfig(this, defaults);
        this.#stopWatching = scrollWatcher(this, { after: floating.after, hideNearEnd: floating.hideNearEnd });
      }
    }

    disconnectedCallback(): void { this.#stopWatching?.(); }

    attributeChangedCallback(): void { if (this.shadowRoot) this.#render(); }

    #render(): void {
      const root = this.shadowRoot ?? this.attachShadow({ mode: 'open' });
      const config = readConfig(this, defaults);
      const page = pageContext();
      const placement = asPlacement(this.getAttribute('placement'));
      const vm = buildShareBar({ catalog }, config, {
        url: this.getAttribute('url') || page.url,
        title: this.getAttribute('title') || page.title,
        site: page.site,
        placement,
        canNativeShare: webShare().available,
      });
      root.innerHTML = renderHtml(vm, config.tracking.attribute, buildCss(config));
      this.#buttons = new Map([...vm.primary, ...vm.secondary].map((b) => [b.key, b]));
      const onClick = handleShareClick(
        { clipboard: browserClipboard(), sharer: webShare(), popup: popupWindow(), notifier: toastNotifier(root), tracker: customEventTracker(this, config.tracking.event_name) },
        config.messages,
      );
      root.querySelector('.w')?.addEventListener('click', (event) => {
        const anchor = event.composedPath().find((n): n is HTMLAnchorElement => n instanceof HTMLAnchorElement);
        const button = anchor && this.#buttons.get(anchor.dataset['key'] ?? '');
        if (button) void onClick({ button, placement, anchor, preventDefault: () => event.preventDefault() });
      });
    }
  }
  if (!customElements.get('promari-sns-share')) customElements.define('promari-sns-share', PromariSnsShareElement);
};
