/** Implements NewTabGateway with a detached browser tab. */
import type { NewTabGateway } from '../domain/gateway/NewTabGateway.ts';

export class BrowserNewTab implements NewTabGateway {
  open(href: string): void {
    window.open(href, '_blank', 'noopener,noreferrer');
  }
}
