/**
 * Browser implementations of the domain gateways. Activity is dispatched to the host
 * page as an event; no telemetry request is sent.
 */
import type { PageInfo } from '../domain/service/policies.ts';
import type { ClipboardGateway, NativeShareGateway, PopupGateway, ShareActivity, ShareActivityPublisher } from '../domain/gateway/ShareGateways.ts';

export const browserClipboard = (): ClipboardGateway => ({
  write: (text) => (navigator.clipboard?.writeText ? navigator.clipboard.writeText(text) : Promise.reject(new Error('clipboard unavailable'))),
  fallback: (text) => { window.prompt('URL', text); },
});

export const webShare = (): NativeShareGateway => ({
  available: typeof navigator.share === 'function',
  share: (data) => navigator.share(data),
});

export const popupWindow = (): PopupGateway => ({
  open: (href, { width, height }) => {
    const left = Math.max(0, (window.screen.width - width) / 2);
    const top = Math.max(0, (window.screen.height - height) / 2);
    return window.open(href, 'promari-sns-share', `width=${width},height=${height},left=${left},top=${top},noopener,noreferrer`) !== null;
  },
});

/**
 * Emit a bubbling, composed CustomEvent that crosses the Shadow DOM boundary.
 */
export const customEventPublisher = (element: HTMLElement, eventName: string): ShareActivityPublisher => ({
  publish: (activity: ShareActivity) => { element.dispatchEvent(new CustomEvent<ShareActivity>(eventName, { bubbles: true, composed: true, detail: activity })); },
});

/**
 * Read the canonical URL, document title, and Open Graph site name.
 */
export const pageContext = (): PageInfo => ({
  url: document.querySelector<HTMLLinkElement>('link[rel="canonical"]')?.href || window.location.href,
  title: document.title,
  site: document.querySelector<HTMLMetaElement>('meta[property="og:site_name"]')?.content ?? '',
});
