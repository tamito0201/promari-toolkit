/**
 * Browser implementations of application ports. Analytics are dispatched to the host page without sending telemetry requests.
 */
import type { ClipboardPort, NotifierPort, PopupPort, SharerPort, TrackDetail, TrackerPort } from '../application/ports.ts';

export const browserClipboard = (): ClipboardPort => ({
  write: (text) => (navigator.clipboard?.writeText ? navigator.clipboard.writeText(text) : Promise.reject(new Error('clipboard unavailable'))),
  fallback: (text) => { window.prompt('URL', text); },
});

export const webShare = (): SharerPort => ({
  available: typeof navigator.share === 'function',
  share: (data) => navigator.share(data),
});

export const popupWindow = (): PopupPort => ({
  open: (href, { width, height }) => {
    const left = Math.max(0, (window.screen.width - width) / 2);
    const top = Math.max(0, (window.screen.height - height) / 2);
    return window.open(href, 'promari-sns-share', `width=${width},height=${height},left=${left},top=${top},noopener,noreferrer`) !== null;
  },
});

/**
 * Show a toast inside the supplied ShadowRoot.
 */
export const toastNotifier = (root: ShadowRoot): NotifierPort => {
  let toast: HTMLDivElement | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  return {
    notify: (text, anchor) => {
      toast ??= Object.assign(root.appendChild(document.createElement('div')), { className: 't' });
      toast.setAttribute('role', 'status');
      toast.textContent = text;
      toast.classList.add('on');
      anchor?.classList.add('done');
      clearTimeout(timer);
      timer = setTimeout(() => { toast?.classList.remove('on'); anchor?.classList.remove('done'); }, 1800);
    },
  };
};

/**
 * Emit a bubbling, composed CustomEvent that crosses the Shadow DOM boundary.
 */
export const customEventTracker = (element: HTMLElement, eventName: string): TrackerPort => ({
  track: (detail: TrackDetail) => { element.dispatchEvent(new CustomEvent<TrackDetail>(eventName, { bubbles: true, composed: true, detail })); },
});

/**
 * Toggle the floating bar after scrolling. Return a cleanup callback.
 */
export const scrollWatcher = (element: HTMLElement, { after, hideNearEnd }: { after: number; hideNearEnd: boolean }): (() => void) => {
  let ticking = false;
  const update = (): void => {
    const y = window.scrollY;
    const end = document.documentElement.scrollHeight - window.innerHeight - 120;
    element.classList.toggle('on', y > after && (!hideNearEnd || y < end));
    ticking = false;
  };
  const onScroll = (): void => { if (!ticking) { ticking = true; requestAnimationFrame(update); } };
  window.addEventListener('scroll', onScroll, { passive: true });
  update();
  return () => window.removeEventListener('scroll', onScroll);
};

/**
 * Read the canonical URL, document title, and Open Graph site name.
 */
export const pageContext = (): { url: string; title: string; site: string } => ({
  url: document.querySelector<HTMLLinkElement>('link[rel="canonical"]')?.href || window.location.href,
  title: document.title,
  site: document.querySelector<HTMLMetaElement>('meta[property="og:site_name"]')?.content ?? '',
});
