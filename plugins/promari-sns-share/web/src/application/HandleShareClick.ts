/**
 * Use case: handle a share click. Domain policies choose the action; gateways defined
 * by the domain perform side effects. The result is returned, and presentation decides
 * how to show it.
 */
import type { ShareGateways } from '../domain/gateway/ShareGateways.ts';
import { decideClick } from '../domain/service/policies.ts';
import type { ButtonViewModel } from './BuildShareBar.ts';

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

export type ClickHandler = (context: ClickContext) => Promise<ShareClickOutcome>;

const perform = async (gateways: ShareGateways, { button, preventDefault }: ClickContext): Promise<ShareClickOutcome> => {
  const decision = decideClick(button);
  switch (decision) {
    case 'copy': {
      preventDefault();
      try {
        await gateways.clipboard.write(button.url);
        return 'copied';
      } catch {
        gateways.clipboard.fallback?.(button.url);
        return 'copy-fallback';
      }
    }
    case 'native': {
      preventDefault();
      return gateways.nativeShare.share({ title: button.title, url: button.url }).then(() => 'shared' as const, () => 'share-dismissed' as const);
    }
    case 'popup': {
      if (button.popup && gateways.popup.open(button.href, button.popup)) { preventDefault(); return 'popup'; }
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
};

export const handleShareClick = (gateways: ShareGateways): ClickHandler => async (context) => {
  const outcome = await perform(gateways, context);
  gateways.activity.publish({ service: context.button.key, url: context.button.url, placement: context.placement });
  return outcome;
};
