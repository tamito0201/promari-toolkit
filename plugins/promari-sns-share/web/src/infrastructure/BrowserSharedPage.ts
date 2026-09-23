/** Implements SharedPageGateway by reading the canonical URL, title, and Open Graph site name. */
import type { SharedPageGateway } from '../domain/gateway/SharedPageGateway.ts';
import type { SharedPage } from '../domain/model/SharedPage.ts';

export class BrowserSharedPage implements SharedPageGateway {
  read(): SharedPage {
    return {
      url: document.querySelector<HTMLLinkElement>('link[rel="canonical"]')?.href || window.location.href,
      title: document.title,
      site: document.querySelector<HTMLMetaElement>('meta[property="og:site_name"]')?.content ?? '',
    };
  }
}
