/**
 * Service entities containing logos, colors, and URL specifications extracted from PHP. URL construction is pure.
 */
import type { Catalog, RequestField, Service, ServiceSpec, ShareRequest } from './types.ts';

const FIELD: Readonly<Record<RequestField, (r: ShareRequest) => string>> = {
  url: (r) => r.url,
  title: (r) => r.title,
  text: (r) => r.text,
  via: (r) => r.via,
  site: (r) => r.site,
  hashtagsCsv: (r) => r.hashtagsCsv,
};

/**
 * Encode queries using RFC 3986 and omit empty values, matching PHP AbstractService::build.
 */
export const buildUrl = (endpoint: string, params: Readonly<Record<string, RequestField>>, request: ShareRequest): string => {
  const pairs = Object.entries(params)
    .map(([name, field]) => [name, FIELD[field](request)] as const)
    .filter(([, value]) => value !== '')
    .map(([name, value]) => `${encodeURIComponent(name)}=${encodeURIComponent(value)}`);
  return endpoint + (pairs.length ? `?${pairs.join('&')}` : '');
};

export const createService = (spec: ServiceSpec): Service => {
  const service: Service = {
    key: spec.key,
    label: spec.label,
    color: spec.color,
    icon: spec.icon,
    action: spec.action,
    /**
 * Return the share dialog URL, or the page URL for copy and native sharing.
 */
    shareUrl: (request: ShareRequest) => (spec.endpoint ? buildUrl(spec.endpoint, spec.params, request) : request.url),
  };
  return Object.freeze(service);
};

export class UnknownServiceError extends Error {
  constructor(key: string) {
    super(`promari-sns-share: 未知のシェア先です: ${key}`);
    this.name = 'UnknownServiceError';
  }
}

/**
 * Immutable registry. Unknown service names throw rather than being silently ignored.
 */
export const createCatalog = (specs: readonly ServiceSpec[]): Catalog => {
  const byKey = new Map(specs.map((spec) => [spec.key, createService(spec)] as const));
  const get = (key: string): Service => byKey.get(key) ?? (() => { throw new UnknownServiceError(key); })();
  const catalog: Catalog = {
    has: (key: string) => byKey.has(key),
    keys: () => [...byKey.keys()],
    resolve: (keys: readonly string[]) => keys.map(get),
  };
  return Object.freeze(catalog);
};
