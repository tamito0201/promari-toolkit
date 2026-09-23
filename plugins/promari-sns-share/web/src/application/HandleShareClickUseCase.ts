/**
 * Use case: handle a share click. The domain decides the action; gateways defined by the
 * domain perform side effects. The outcome is returned, and presentation decides how to show it.
 */
import type { ShareGateways } from '../domain/gateway/ShareGateways.ts';
import { ClickPolicy } from '../domain/service/ClickPolicy.ts';
import type { ButtonViewModel } from './BuildShareBarUseCase.ts';

export interface ClickContext {
  readonly button: ButtonViewModel;
  readonly placement: string;
  preventDefault(): void;
}

/**
 * What the click achieved, as far as this page can observe.
 * - copied: the clipboard accepted the text
 * - copy-fallback: writing failed, so the fallback was offered
 * - shared / share-dismissed: the native share sheet finished or was dismissed
 * - popup: a share window opened
 * - follow: the browser follows the link (including a blocked popup)
 */
export type ShareClickOutcome = 'copied' | 'copy-fallback' | 'shared' | 'share-dismissed' | 'popup' | 'follow';

export class HandleShareClickUseCase {
  readonly #gateways: ShareGateways;

  constructor(gateways: ShareGateways) {
    this.#gateways = gateways;
  }

  async execute(context: ClickContext): Promise<ShareClickOutcome> {
    const outcome = await this.#perform(context);
    this.#gateways.activity.publish({ service: context.button.key, url: context.button.url, placement: context.placement });
    return outcome;
  }

  async #perform({ button, preventDefault }: ClickContext): Promise<ShareClickOutcome> {
    const decision = ClickPolicy.decide(button);
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
      case 'popup': {
        if (button.popup && this.#gateways.popup.open(button.href, button.popup)) { preventDefault(); return 'popup'; }
        return 'follow';
      }
      case 'follow':
        return 'follow'; // Let the browser follow the link.
      default: {
        // ClickDecision を増やしたらここでコンパイルが止まる。黙って素通りさせない。
        const unhandled: never = decision;
        throw new Error(`promari-sns-share: 未対応のクリック操作です: ${String(unhandled)}`);
      }
    }
  }
}
