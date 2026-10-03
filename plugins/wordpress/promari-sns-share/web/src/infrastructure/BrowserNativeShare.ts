/** Implements NativeShareGateway with the Web Share API. */
import type { NativeShareGateway } from '../domain/gateway/NativeShareGateway.ts';

export class BrowserNativeShare implements NativeShareGateway {
  get available(): boolean {
    return typeof navigator.share === 'function';
  }

  share(data: { title: string; url: string }): Promise<void> {
    return navigator.share(data);
  }
}
