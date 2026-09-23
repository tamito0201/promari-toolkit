/**
 * The outside capabilities that sharing needs. The domain states what it needs;
 * infrastructure implements each interface with browser APIs, and tests supply plain objects.
 */
import type { ClipboardGateway } from './ClipboardGateway.ts';
import type { NativeShareGateway } from './NativeShareGateway.ts';
import type { ShareWindowGateway } from './ShareWindowGateway.ts';
import type { ShareActivityPublisher } from './ShareActivityPublisher.ts';

export interface ShareGateways {
  readonly clipboard: ClipboardGateway;
  readonly nativeShare: NativeShareGateway;
  readonly popup: ShareWindowGateway;
  readonly activity: ShareActivityPublisher;
}
