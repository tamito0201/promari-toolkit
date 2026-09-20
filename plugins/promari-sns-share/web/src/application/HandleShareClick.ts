/**
 * Handle share clicks. Domain policies choose the action; infrastructure ports perform side effects.
 */
import { decideClick } from '../domain/policies.ts';
import type { ButtonViewModel } from './BuildShareBar.ts';
import type { Ports } from './ports.ts';

export interface ClickContext {
  readonly button: ButtonViewModel;
  readonly placement: string;
  /** 描画層で解決する通知先の識別子。DOMノードは渡さない。 */
  readonly notificationTarget?: string;
  preventDefault(): void;
}

export type ClickHandler = (context: ClickContext) => Promise<void>;

export const handleShareClick = (ports: Ports, messages: { readonly copied: string }): ClickHandler =>
  async ({ button, placement, notificationTarget, preventDefault }) => {
    const decision = decideClick(button);
    switch (decision) {
      case 'copy': {
        preventDefault();
        try {
          await ports.clipboard.write(button.url);
          ports.notifier.notify(messages.copied, notificationTarget);
        } catch {
          ports.clipboard.fallback?.(button.url);
        }
        break;
      }
      case 'native': {
        preventDefault();
        await ports.sharer.share({ title: button.title, url: button.url }).catch(() => undefined);
        break;
      }
      case 'popup': {
        if (button.popup && ports.popup.open(button.href, button.popup)) preventDefault();
        break;
      }
      case 'follow':
        break; // Let the browser follow the link.
      default: {
        // ClickDecision を増やしたらここでコンパイルが止まる。黙って素通りさせない。
        const unhandled: never = decision;
        throw new Error(`promari-sns-share: 未対応のクリック操作です: ${String(unhandled)}`);
      }
    }
    ports.tracker.track({ service: button.key, url: button.url, placement });
  };
