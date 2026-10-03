/** Implements ClipboardGateway with the browser Clipboard API. */
import type { ClipboardGateway } from '../domain/gateway/ClipboardGateway.ts';

export class BrowserClipboard implements ClipboardGateway {
  write(text: string): Promise<void> {
    return navigator.clipboard?.writeText ? navigator.clipboard.writeText(text) : Promise.reject(new Error('clipboard unavailable'));
  }

  fallback(text: string): void {
    window.prompt('URL', text);
  }
}
