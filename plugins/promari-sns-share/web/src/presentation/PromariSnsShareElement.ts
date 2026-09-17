/**
 * The promari-sns-share custom element. Read attributes, call use cases, render into Shadow DOM, and forward clicks. Policies belong to domain; browser effects go through ports.
 */
import { buildShareBar, type ButtonViewModel } from '../application/BuildShareBar.ts';
import { handleShareClick } from '../application/HandleShareClick.ts';
import type { Catalog, Placement, ShareConfig } from '../domain/types.ts';
import { OBSERVED_ATTRIBUTES, readConfig } from '../infrastructure/AttributeConfig.ts';
import { browserClipboard, customEventTracker, pageContext, popupWindow, scrollWatcher, toastNotifier, webShare } from '../infrastructure/browserPorts.ts';
import { buildCss } from './styles.ts';
import { renderCircle } from './circle.ts';
import { renderHtml } from './view.ts';

const PLACEMENTS: ReadonlySet<string> = new Set<Placement>(['article_top', 'article_bottom', 'sidebar', 'floating', 'inline']);
const asPlacement = (value: string | null): Placement => (value && PLACEMENTS.has(value) ? (value as Placement) : 'inline');

export interface ElementDeps {
  readonly catalog: Catalog;
  readonly defaults: ShareConfig;
}

export const defineElement = ({ catalog, defaults }: ElementDeps): void => {
  class PromariSnsShareElement extends HTMLElement {
    static get observedAttributes(): readonly string[] { return [...OBSERVED_ATTRIBUTES, 'variant', 'caption', 'like']; }

    #interactions = new AbortController();
    #likeState = { liked: false, count: 0, busy: true, message: '' };
    setLikeState(state: { liked: boolean; count: number; busy?: boolean; message?: string }): void {
      if (!Number.isSafeInteger(state.count) || state.count < 0) return;
      this.#likeState = { liked: state.liked, count: state.count, busy: state.busy ?? false, message: state.message ?? '' };
      this.#updateLike();
    }
    #updateLike(): void {
      const root = this.shadowRoot;
      const like = root?.querySelector<HTMLButtonElement>('.like');
      if (like) {
        like.disabled = this.#likeState.busy;
        like.setAttribute('aria-pressed', String(this.#likeState.liked));
        root!.querySelector('.like-label')!.textContent = this.#likeState.liked ? 'いいね済み' : 'いいね';
        root!.querySelector('.count')!.textContent = String(this.#likeState.count);
      }
      const status = root?.querySelector('.status');
      if (status) status.textContent = this.#likeState.message;
    }
    #buttons = new Map<string, ButtonViewModel>();
    #stopWatching: (() => void) | null = null;

    connectedCallback(): void {
      this.#render();
      if (asPlacement(this.getAttribute('placement')) === 'floating') {
        const { floating } = readConfig(this, defaults);
        this.#stopWatching = scrollWatcher(this, { after: floating.after, hideNearEnd: floating.hideNearEnd });
      }
    }

    disconnectedCallback(): void { this.#stopWatching?.(); this.#interactions.abort(); }

    attributeChangedCallback(): void { if (this.shadowRoot) this.#render(); }

    #render(): void {
      this.#interactions.abort();
      this.#interactions = new AbortController();
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
      const circle = this.getAttribute('variant') === 'circle';
      root.innerHTML = circle
        ? renderCircle(vm, config.tracking.attribute, this.getAttribute('caption') ?? '', this.hasAttribute('like'))
        : renderHtml(vm, config.tracking.attribute, buildCss(config));
      this.#updateLike();
      root.querySelector('.like')?.addEventListener('click', () => {
        if (this.#likeState.busy) return;
        this.#likeState.busy = true;
        this.#updateLike();
        this.dispatchEvent(new CustomEvent('promari-sns-share-like', { bubbles: true, composed: true, detail: { liked: !this.#likeState.liked } }));
      });
      const details = root.querySelector('details');
      this.ownerDocument.addEventListener('pointerdown', (event) => {
        if (details && !event.composedPath().includes(details)) details.open = false;
      }, { signal: this.#interactions.signal });
      details?.addEventListener('keydown', (event) => {
        if (event.key === 'Escape') { details.open = false; details.querySelector('summary')?.focus(); }
      });
      details?.addEventListener('toggle', () => {
        const panel = details.querySelector<HTMLElement>('.options');
        if (!details.open || !panel) return;
        panel.style.transform = '';
        const box = panel.getBoundingClientRect();
        const shift = box.left < 16 ? 16 - box.left : Math.min(0, innerWidth - 16 - box.right);
        panel.style.transform = `translateX(${shift}px)`;
      });
      this.#buttons = new Map([...vm.primary, ...vm.secondary].map((b) => [b.key, b]));
      const onClick = handleShareClick(
        { clipboard: browserClipboard(), sharer: webShare(), popup: popupWindow(), notifier: toastNotifier(root), tracker: customEventTracker(this, config.tracking.event_name) },
        config.messages,
      );
      root.querySelector(circle ? '.circle' : '.w')?.addEventListener('click', (event) => {
        const anchor = event.composedPath().find((n): n is HTMLAnchorElement | HTMLButtonElement => n instanceof HTMLAnchorElement || n instanceof HTMLButtonElement);
        const button = anchor && this.#buttons.get(anchor.dataset['key'] ?? '');
        if (button?.action === 'native' && !webShare().available && details) {
          event.preventDefault(); details.open = true; details.querySelector('summary')?.focus(); return;
        }
        if (button) void onClick({ button, placement, anchor, preventDefault: () => event.preventDefault() });
      });
    }
  }
  if (!customElements.get('promari-sns-share')) customElements.define('promari-sns-share', PromariSnsShareElement);
};
