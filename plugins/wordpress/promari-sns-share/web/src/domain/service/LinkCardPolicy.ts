/**
 * Domain service: keep the values in a link-card draft, and the share URL that carries it, within
 * the limits measured on 2026-10-01 (ADR-0004). Ameba Blog's limit applies to the whole request, URL plus
 * cookies: signed in with about 1,800 characters of cookies, 5,105 characters of URL passed and 5,140 got
 * 400. Share URLs that carry a draft stay at 3,500 or below, which still passes for a reader with about
 * 1,600 more characters of cookies than measured.
 */
import type { ShareRequest } from '../model/ShareRequest.ts';

const ELLIPSIS = '…';

export class LinkCardPolicy {
  static readonly TITLE_MAX = 100;
  static readonly DESCRIPTION_MAX = 60;
  static readonly URL_MAX = 3500;

  /**
   * Shorten a value to at most `max` characters (code points, so no surrogate pair is split),
   * ending with "…" when anything was cut. Zero or less gives an empty string.
   */
  static clip(value: string, max: number): string {
    const chars = Array.from(value);
    if (chars.length <= max) return value;
    return max <= 0 ? '' : chars.slice(0, max - 1).join('') + ELLIPSIS;
  }

  /**
   * The request as a card shows it: whitespace (such as line breaks in a meta description) collapsed,
   * the title cut to 100 characters, and the description to 60.
   */
  static bound(request: ShareRequest): ShareRequest {
    const tidy = (value: string): string => value.replace(/\s+/g, ' ').trim();
    return request.withPreview({
      title: LinkCardPolicy.clip(tidy(request.title), LinkCardPolicy.TITLE_MAX),
      description: LinkCardPolicy.clip(tidy(request.description), LinkCardPolicy.DESCRIPTION_MAX),
    });
  }

  /**
   * Build the share URL, degrading the card until the URL is at most `limit` characters: first the
   * description is shortened and then left out, then the image is left out, and last the title is
   * shortened. A Japanese character costs nine characters once percent-encoded and the title appears
   * in both the card and the post title, so dropping the description and image alone does not bring
   * a long Japanese title under the limit. If even an empty title does not fit (a page URL of about
   * a thousand characters), the shortest URL is returned: nothing else can be removed.
   */
  static fit(request: ShareRequest, build: (request: ShareRequest) => string, limit: number = LinkCardPolicy.URL_MAX): string {
    const fits = (candidate: ShareRequest): boolean => build(candidate).length <= limit;
    if (fits(request)) return build(request);
    const shortDescription = LinkCardPolicy.#shorten(request, 'description', fits);
    if (fits(shortDescription)) return build(shortDescription);
    const noImage = shortDescription.withPreview({ image: '' });
    if (fits(noImage)) return build(noImage);
    return build(LinkCardPolicy.#shorten(noImage, 'title', fits));
  }

  /** The longest clip of one value that fits, found by bisection; an empty value when none does. */
  static #shorten(request: ShareRequest, field: 'title' | 'description', fits: (candidate: ShareRequest) => boolean): ShareRequest {
    const value = request[field];
    const at = (max: number): ShareRequest => request.withPreview({ [field]: LinkCardPolicy.clip(value, max) });
    let low = 0; // at(low) is the fallback: an empty value
    let high = Array.from(value).length - 1; // the full value is known not to fit
    while (low < high) {
      const middle = Math.ceil((low + high) / 2);
      if (fits(at(middle))) low = middle; else high = middle - 1;
    }
    return at(low);
  }
}
