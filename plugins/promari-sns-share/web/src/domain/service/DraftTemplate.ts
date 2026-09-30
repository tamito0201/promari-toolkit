/**
 * Domain service: fill in a destination's draft template (ADR-0004).
 */
import type { DraftSpec } from '../model/ShareDestinationSpec.ts';
import type { ShareRequest } from '../model/ShareRequest.ts';

const PLACEHOLDER = /\{(?:title|url|description|image|host)\}/g;

const HTML_ENTITIES: Readonly<Record<string, string>> = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };

export class DraftTemplate {
  /** The draft a compose destination copies when it declares no template: title, line break, URL (ADR-0003). */
  static readonly COMPOSE_DEFAULT: DraftSpec = Object.freeze({ template: '{title}\n{url}', format: 'text' });

  /**
   * Fill in the placeholders in a single pass; inserted values are never scanned again, so a title
   * containing "{url}" stays a title. `html` escapes every value, `text` inserts it as is. A text draft
   * never starts with blank lines, so an empty title does not leave the paste starting with a gap.
   */
  static render({ template, format }: DraftSpec, request: ShareRequest): string {
    const values: Readonly<Record<string, string>> = {
      '{title}': request.title.trim(),
      '{url}': request.url,
      '{description}': request.description,
      '{image}': request.image,
      '{host}': DraftTemplate.host(request.url),
    };
    const encode = format === 'html' ? DraftTemplate.escapeHtml : (value: string): string => value;
    const draft = template.replace(PLACEHOLDER, (placeholder) => encode(values[placeholder] ?? ''));
    return format === 'text' ? draft.replace(/^\s+/, '') : draft;
  }

  /** The host name of an absolute URL, without credentials or port; empty when there is none. */
  static host(url: string): string {
    const authority = /^[a-z][a-z0-9+.-]*:\/\/([^/?#]*)/i.exec(url)?.[1] ?? '';
    const withoutUser = authority.slice(authority.lastIndexOf('@') + 1);
    return withoutUser.startsWith('[') ? withoutUser.slice(0, withoutUser.indexOf(']') + 1) : withoutUser.replace(/:\d*$/, '');
  }

  static escapeHtml(value: string): string {
    return value.replace(/[&<>"']/g, (char) => HTML_ENTITIES[char] ?? char);
  }
}
