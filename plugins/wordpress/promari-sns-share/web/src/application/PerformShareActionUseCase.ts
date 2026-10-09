/**
 * Use case: perform the share action for a button (copy, native share, compose, popup, or follow).
 * The domain decides the action; gateways defined by the domain perform side effects. The outcome is
 * returned, and presentation decides how to show it. Named after the domain's ShareAction, not the
 * click that triggers it: the click belongs to presentation.
 */
import type { ShareGateways } from '../domain/gateway/ShareGateways.ts';
import { ShareActionPolicy } from '../domain/service/ShareActionPolicy.ts';
import type { ShareButtonViewModel } from './BuildShareBarUseCase.ts';

export interface ShareActionCommand {
  readonly button: ShareButtonViewModel;
  readonly placement: string;
  preventDefault(): void;
}

/**
 * What the share action achieved, as far as this page can observe.
 * - copied: the clipboard accepted the text
 * - copy-fallback: writing failed, so the fallback was offered
 * - shared / share-dismissed: the native share sheet finished or was dismissed
 * - composed / compose-copy-failed: the editor tab was requested, and the title and URL were
 *   (or could not be) put on the clipboard
 * - popup: a share window opened
 * - follow: the browser follows the link (including a blocked popup)
 */
export type ShareActionResult = 'copied' | 'copy-fallback' | 'shared' | 'share-dismissed' | 'composed' | 'compose-copy-failed' | 'popup' | 'follow';

export class PerformShareActionUseCase {
  readonly #gateways: ShareGateways;

  constructor(gateways: ShareGateways) {
    this.#gateways = gateways;
  }

  async execute(context: ShareActionCommand): Promise<ShareActionResult> {
    const outcome = await this.#perform(context);
    this.#gateways.activity.publish({ destination: context.button.key, url: context.button.url, placement: context.placement });
    return outcome;
  }

  async #perform({ button, preventDefault }: ShareActionCommand): Promise<ShareActionResult> {
    const decision = ShareActionPolicy.decide(button);
    switch (decision) {
      case 'copy': {
        preventDefault();
        try {
          await this.#gateways.clipboard.write(button.url);
          return 'copied';
        } catch {
          this.#gateways.clipboard.fallback?.(button.url);
          return 'copy-fallback';
        }
      }
      case 'native': {
        preventDefault();
        return this.#gateways.nativeShare.share({ title: button.title, url: button.url }).then(() => 'shared' as const, () => 'share-dismissed' as const);
      }
      case 'compose': {
        preventDefault();
        // Popup blockers honour window.open only in the click's synchronous turn, so both effects start
        // before the first await. The clipboard write starts first; a synchronous throw becomes a rejection
        // so the editor still opens. A failed copy is reported, not retried with a prompt: the user is
        // already looking at the new tab (ADR-0003).
        const writing = new Promise<void>((resolve) => { resolve(this.#gateways.clipboard.write(button.draft)); });
        this.#gateways.newTab.open(button.href);
        return writing.then(() => 'composed' as const, () => 'compose-copy-failed' as const);
      }
      case 'popup': {
        if (button.popup && this.#gateways.popup.open(button.href, button.popup)) { preventDefault(); return 'popup'; }
        return 'follow';
      }
      case 'follow':
        return 'follow'; // Let the browser follow the link.
      default: {
        // ShareActionDecision を増やしたらここでコンパイルが止まる。黙って素通りさせない。
        const unhandled: never = decision;
        throw new Error(`promari-sns-share: 未対応のクリック操作です: ${String(unhandled)}`);
      }
    }
  }
}
