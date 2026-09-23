/** Implements PopupGateway with a centered browser window. */
import type { PopupGateway } from '../domain/gateway/PopupGateway.ts';

export class PopupWindowGateway implements PopupGateway {
  open(href: string, { width, height }: { width: number; height: number }): boolean {
    const left = Math.max(0, (window.screen.width - width) / 2);
    const top = Math.max(0, (window.screen.height - height) / 2);
    return window.open(href, 'promari-sns-share', `width=${width},height=${height},left=${left},top=${top},noopener,noreferrer`) !== null;
  }
}
