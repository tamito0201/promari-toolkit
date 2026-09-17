/**
 * Immutable shared-content value object matching the PHP ShareRequest. URLs already include UTM parameters and text is already formatted.
 */
import type { ShareRequest } from './types.ts';

export interface ShareRequestInput {
  readonly url: string;
  readonly title: string;
  readonly text: string;
  readonly hashtags?: readonly string[];
  readonly via?: string;
  readonly site?: string;
}

export const createShareRequest = ({ url, title, text, hashtags = [], via = '', site = '' }: ShareRequestInput): ShareRequest => {
  const tags = Object.freeze(hashtags.map(String).filter(Boolean));
  return Object.freeze({ url, title, text, hashtags: tags, via: via.replace(/^@/, ''), site, hashtagsCsv: tags.join(',') });
};

/**
 * Copy the request with a service-specific URL.
 */
export const withUrl = (request: ShareRequest, url: string): ShareRequest => createShareRequest({ ...request, url });
