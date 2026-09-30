/**
 * Share destination value object. It holds the meaning of a destination and its URL rule;
 * display metadata such as labels and colors belongs to the application layer.
 *
 * 共有URLを組み立てる値オブジェクト。key が同じなら振る舞いも同じで、
 * 状態も同一性も持たないため、DDD でいう Entity ではない。
 */
import { UriEncoder } from '../service/UriEncoder.ts';
import { DraftTemplate } from '../service/DraftTemplate.ts';
import { LinkCardPolicy } from '../service/LinkCardPolicy.ts';
import type { ShareRequest } from './ShareRequest.ts';
import { ShareAction } from './ShareAction.ts';
import type { DraftSpec, ShareRequestField, ShareDestinationSpec } from './ShareDestinationSpec.ts';

type RequestValue = Exclude<ShareRequestField, 'draft'>;

const FIELD: Readonly<Record<RequestValue, (r: ShareRequest) => string>> = {
  url: (r) => r.url,
  title: (r) => r.title,
  text: (r) => r.text,
  via: (r) => r.via,
  site: (r) => r.site,
  hashtagsCsv: (r) => r.hashtagsCsv,
};

export class ShareDestination {
  readonly key: string;
  readonly action: ShareAction;
  readonly #endpoint: string;
  readonly #params: Readonly<Record<string, ShareRequestField>>;
  readonly #draft: DraftSpec | undefined;

  constructor({ key, action, endpoint, params, draft }: ShareDestinationSpec) {
    this.key = key;
    this.action = action;
    this.#endpoint = endpoint;
    this.#params = params;
    this.#draft = draft;
    Object.freeze(this);
  }

  /** Whether the share URL carries the draft, prefilling the service's editor (ADR-0004). */
  get sendsDraft(): boolean {
    return Object.values(this.#params).includes('draft');
  }

  /**
   * Return the share dialog URL, or the page URL for copy and native sharing.
   * Queries are encoded with RFC 3986 and empty values are omitted. A URL that carries a draft is
   * kept within the link-card limit by degrading the card (LinkCardPolicy.fit).
   */
  shareUrl(request: ShareRequest): string {
    if (!this.#endpoint) return request.url;
    if (!this.sendsDraft) return this.#query(request);
    return LinkCardPolicy.fit(LinkCardPolicy.bound(request), (candidate) => this.#query(candidate));
  }

  /**
   * The text a compose destination puts on the clipboard: the destination's draft template filled in
   * (ADR-0004), or without one, the page title, a line break, and the shared URL (ADR-0003). A text
   * draft never starts with a blank line, so without a title only the URL is copied. Other actions
   * copy nothing of their own and return an empty string.
   */
  composeDraft(request: ShareRequest): string {
    return this.action === ShareAction.Compose ? this.#render(request) : '';
  }

  #render(request: ShareRequest): string {
    return this.#draft
      ? DraftTemplate.render(this.#draft, LinkCardPolicy.bound(request))
      : DraftTemplate.render(DraftTemplate.COMPOSE_DEFAULT, request);
  }

  #query(request: ShareRequest): string {
    const pairs = Object.entries(this.#params)
      .map(([name, field]) => [name, field === 'draft' ? this.#render(request) : FIELD[field](request)] as const)
      .filter(([, value]) => value !== '')
      .map(([name, value]) => `${UriEncoder.encode(name)}=${UriEncoder.encode(value)}`);
    return this.#endpoint + (pairs.length ? `?${pairs.join('&')}` : '');
  }
}
