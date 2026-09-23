/**
 * Domain service: add UTM parameters to the shared URL.
 */
import type { UtmConfig } from '../model/ShareTypes.ts';
import { UriEncoder } from './UriEncoder.ts';

export class UtmPolicy {
  /** Add UTM parameters when enabled and expand the service placeholder. */
  static apply(url: string, utm: UtmConfig, service: string): string {
    if (!utm.enabled) return url;
    const params = Object.fromEntries(
      (['source', 'medium', 'campaign', 'content'] as const)
        .map((k) => [`utm_${k}`, utm[k].replaceAll('{service}', service)] as const)
        .filter(([, v]) => v !== ''),
    );
    return Object.keys(params).length ? UriEncoder.appendQuery(url, params) : url;
  }
}
