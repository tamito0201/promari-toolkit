/**
 * Share service value object. It holds the meaning of a service and its URL rule;
 * display metadata such as labels and colors belongs to the application layer.
 */
import { encodeComponent } from '../service/encoding.ts';
import type { Action, RequestField, ServiceSpec } from './types.ts';
import type { ShareRequest } from './ShareRequest.ts';

/**
 * 共有URLを組み立てる値オブジェクト。key が同じなら振る舞いも同じで、
 * 状態も同一性も持たないため、DDD でいう Entity ではない。
 */
export interface ShareService {
  readonly key: string;
  readonly action: Action;
  shareUrl(request: ShareRequest): string;
}

const FIELD: Readonly<Record<RequestField, (r: ShareRequest) => string>> = {
  url: (r) => r.url,
  title: (r) => r.title,
  text: (r) => r.text,
  via: (r) => r.via,
  site: (r) => r.site,
  hashtagsCsv: (r) => r.hashtagsCsv,
};

/**
 * Encode queries using RFC 3986 and omit empty values.
 */
export const buildUrl = (endpoint: string, params: Readonly<Record<string, RequestField>>, request: ShareRequest): string => {
  const pairs = Object.entries(params)
    .map(([name, field]) => [name, FIELD[field](request)] as const)
    .filter(([, value]) => value !== '')
    .map(([name, value]) => `${encodeComponent(name)}=${encodeComponent(value)}`);
  return endpoint + (pairs.length ? `?${pairs.join('&')}` : '');
};

/**
 * Factory for the value object. Return the share dialog URL, or the page URL for copy and native sharing.
 */
export const createShareService = ({ key, action, endpoint, params }: ServiceSpec): ShareService =>
  Object.freeze({
    key,
    action,
    shareUrl: (request: ShareRequest) => (endpoint ? buildUrl(endpoint, params, request) : request.url),
  });
