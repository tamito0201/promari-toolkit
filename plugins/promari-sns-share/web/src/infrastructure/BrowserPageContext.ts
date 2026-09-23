/** Implements PageContextGateway by reading the canonical URL, title, and Open Graph site name. */
import type { PageContextGateway } from '../domain/gateway/PageContextGateway.ts';
import type { PageInfo } from '../domain/model/ShareTypes.ts';

export class BrowserPageContext implements PageContextGateway {
  read(): PageInfo {
    return {
      url: document.querySelector<HTMLLinkElement>('link[rel="canonical"]')?.href || window.location.href,
      title: document.title,
      site: document.querySelector<HTMLMetaElement>('meta[property="og:site_name"]')?.content ?? '',
    };
  }
}
