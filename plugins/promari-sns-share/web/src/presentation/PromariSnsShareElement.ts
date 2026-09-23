/**
 * The promari-sns-share custom element: the presentation layer. Read attributes, call use cases,
 * render into Shadow DOM, and show the outcome. Policies belong to domain; browser effects go
 * through gateways implemented in infrastructure and passed in at startup.
 */
import type { BuildShareBarUseCase, ButtonViewModel, Placement } from '../application/BuildShareBarUseCase.ts';
import type { HandleShareClickUseCase } from '../application/HandleShareClickUseCase.ts';
import type { ShareConfig } from '../application/ShareConfig.ts';
import { AttributeConfigReader } from './AttributeConfigReader.ts';
import { CircleView } from './CircleView.ts';
import { FloatingVisibility } from './FloatingVisibility.ts';
import { ShareBarStyles } from './ShareBarStyles.ts';
import { ShareBarView } from './ShareBarView.ts';
import { ToastNotifier } from './ToastNotifier.ts';

const PLACEMENTS: ReadonlySet<string> = new Set<Placement>(['article_top', 'article_bottom', 'sidebar', 'floating', 'inline']);
const asPlacement = (value: string | null): Placement => (value && PLACEMENTS.has(value) ? (value as Placement) : 'inline');

/** ページとブラウザへの接続は入口から注入する。表示層は接続先の実装をimportしない。 */
export interface ElementEnvironment {
  pageContext(): { readonly url: string; readonly title: string; readonly site: string };
  canNativeShare(): boolean;
  /** The click use case with gateways whose activity events use this name. */
  clickUseCase(eventName: string): HandleShareClickUseCase;
}
export interface ElementDeps {
  readonly buildShareBar: BuildShareBarUseCase;
  readonly defaults: ShareConfig;
  connect(element: HTMLElement): ElementEnvironment;
}

export class PromariSnsShareElement extends HTMLElement {
  static #registered: ElementDeps | undefined;

  /** Register the element with the use cases and page connections chosen at startup. */
  static define(deps: ElementDeps): void {
    PromariSnsShareElement.#registered = deps;
    if (!customElements.get('promari-sns-share')) customElements.define('promari-sns-share', PromariSnsShareElement);
  }

  static get observedAttributes(): readonly string[] { return [...AttributeConfigReader.observedAttributes, 'variant', 'caption', 'like']; }

  static #required(): ElementDeps {
    if (!PromariSnsShareElement.#registered) throw new Error('promari-sns-share: define() を先に呼んでください');
    return PromariSnsShareElement.#registered;
  }

  readonly #deps: ElementDeps = PromariSnsShareElement.#required();
  readonly #config = new AttributeConfigReader(this.#deps.defaults);
  #environment: ElementEnvironment = this.#deps.connect(this);
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
  #notificationSequence = 0;
  #buttons = new Map<string, ButtonViewModel>();
  #floating: FloatingVisibility | null = null;

  connectedCallback(): void {
    this.#render();
    if (asPlacement(this.getAttribute('placement')) === 'floating') {
      this.#floating = new FloatingVisibility(this, this.#config.read(this).floating);
      this.#floating.start();
    }
  }

  disconnectedCallback(): void { this.#floating?.stop(); this.#interactions.abort(); }

  attributeChangedCallback(): void { if (this.shadowRoot) this.#render(); }

  #render(): void {
    this.#interactions.abort();
    this.#interactions = new AbortController();
    const root = this.shadowRoot ?? this.attachShadow({ mode: 'open' });
    const config = this.#config.read(this);
    const page = this.#environment.pageContext();
    const placement = asPlacement(this.getAttribute('placement'));
    const vm = this.#deps.buildShareBar.execute(config, {
      url: this.getAttribute('url') || page.url,
      title: this.getAttribute('title') || page.title,
      site: page.site,
      placement,
      canNativeShare: this.#environment.canNativeShare(),
    });
    const circle = this.getAttribute('variant') === 'circle';
    root.innerHTML = circle
      ? CircleView.render(vm, config.tracking.attribute, this.getAttribute('caption') ?? '', this.hasAttribute('like'))
      : ShareBarView.render(vm, config.tracking.attribute, ShareBarStyles.build(config));
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
    const clickUseCase = this.#environment.clickUseCase(config.tracking.event_name);
    const notifier = new ToastNotifier(root);
    root.querySelector(circle ? '.circle' : '.w')?.addEventListener('click', (event) => {
      const anchor = event.composedPath().find((n): n is HTMLAnchorElement | HTMLButtonElement => n instanceof HTMLAnchorElement || n instanceof HTMLButtonElement);
      const button = anchor && this.#buttons.get(anchor.dataset['key'] ?? '');
      if (button?.action === 'native' && !this.#environment.canNativeShare() && details) {
        event.preventDefault(); details.open = true; details.querySelector('summary')?.focus(); return;
      }
      if (button && anchor) {
        // 同じ共有先が複数ある場合も、押された要素を識別する。
        const notificationTarget = String(++this.#notificationSequence);
        anchor.dataset['notificationTarget'] = notificationTarget;
        void clickUseCase.execute({ button, placement, preventDefault: () => event.preventDefault() }).then((outcome) => {
          if (outcome === 'copied') notifier.notify(config.messages.copied, notificationTarget);
        });
      }
    });
  }
}
