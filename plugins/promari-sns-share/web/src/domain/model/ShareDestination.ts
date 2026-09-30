/**
 * Share destination value object. It holds the meaning of a destination and its URL rule;
 * display metadata such as labels and colors belongs to the application layer.
 *
 * 共有URLを組み立てる値オブジェクト。key が同じなら振る舞いも同じで、
 * 状態も同一性も持たないため、DDD でいう Entity ではない。
 */
import { UriEncoder } from '../service/UriEncoder.ts';
import type { ShareRequest } from './ShareRequest.ts';
import { ShareAction } from './ShareAction.ts';
import type { ShareRequestField, ShareDestinationSpec } from './ShareDestinationSpec.ts';

const FIELD: Readonly<Record<ShareRequestField, (r: ShareRequest) => string>> = {
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

  constructor({ key, action, endpoint, params }: ShareDestinationSpec) {
    this.key = key;
    this.action = action;
    this.#endpoint = endpoint;
    this.#params = params;
    Object.freeze(this);
  }

  /**
   * Return the share dialog URL, or the page URL for copy and native sharing.
   * Queries are encoded with RFC 3986 and empty values are omitted.
   */
  shareUrl(request: ShareRequest): string {
    if (!this.#endpoint) return request.url;
    const pairs = Object.entries(this.#params)
      .map(([name, field]) => [name, FIELD[field](request)] as const)
      .filter(([, value]) => value !== '')
      .map(([name, value]) => `${UriEncoder.encode(name)}=${UriEncoder.encode(value)}`);
    return this.#endpoint + (pairs.length ? `?${pairs.join('&')}` : '');
  }

  /**
   * The text a compose destination puts on the clipboard: the page title, a line break, and the
   * shared URL (ADR-0003). Without a title only the URL is copied, so the paste never starts with a
   * blank line. Other actions copy nothing of their own and return an empty string.
   */
  composeDraft(request: ShareRequest): string {
    if (this.action !== ShareAction.Compose) return '';
    const heading = request.title.trim();
    return heading ? `${heading}\n${request.url}` : request.url;
  }
}
