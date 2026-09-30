/**
 * Implements SharedPageGateway by reading the canonical URL, title, Open Graph site name, and the
 * description and image that link-card drafts show (ADR-0004).
 */
import type { SharedPageGateway } from '../domain/gateway/SharedPageGateway.ts';
import type { SharedPage } from '../domain/model/SharedPage.ts';

const meta = (selector: string): string => document.querySelector<HTMLMetaElement>(selector)?.content.trim() ?? '';

export class BrowserSharedPage implements SharedPageGateway {
  read(): SharedPage {
    return {
      url: document.querySelector<HTMLLinkElement>('link[rel="canonical"]')?.href || window.location.href,
      title: document.title,
      site: document.querySelector<HTMLMetaElement>('meta[property="og:site_name"]')?.content ?? '',
      description: meta('meta[property="og:description"]') || meta('meta[name="description"]'),
      image: BrowserSharedPage.absoluteHttpUrl(meta('meta[property="og:image"]')),
    };
  }

  /** Resolve a relative image URL against the document; anything but http(s) becomes empty (fail closed). */
  static absoluteHttpUrl(value: string): string {
    if (!value) return '';
    try {
      const url = new URL(value, document.baseURI);
      return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : '';
    } catch {
      return '';
    }
  }
}
