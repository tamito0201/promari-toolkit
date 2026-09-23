/**
 * Share service value object. It holds the meaning of a service and its URL rule;
 * display metadata such as labels and colors belongs to the application layer.
 *
 * 共有URLを組み立てる値オブジェクト。key が同じなら振る舞いも同じで、
 * 状態も同一性も持たないため、DDD でいう Entity ではない。
 */
import { UriEncoder } from '../service/UriEncoder.ts';
import type { ShareRequest } from './ShareRequest.ts';
import type { Action, RequestField, ServiceSpec } from './ShareTypes.ts';

const FIELD: Readonly<Record<RequestField, (r: ShareRequest) => string>> = {
  url: (r) => r.url,
  title: (r) => r.title,
  text: (r) => r.text,
  via: (r) => r.via,
  site: (r) => r.site,
  hashtagsCsv: (r) => r.hashtagsCsv,
};

export class ShareService {
  readonly key: string;
  readonly action: Action;
  readonly #endpoint: string;
  readonly #params: Readonly<Record<string, RequestField>>;

  constructor({ key, action, endpoint, params }: ServiceSpec) {
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
}
