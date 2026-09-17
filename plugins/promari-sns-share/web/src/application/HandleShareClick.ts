/**
 * Handle share clicks. Domain policies choose the action; infrastructure ports perform side effects.
 */
import { decideClick } from '../domain/policies.ts';
import type { ButtonViewModel } from './BuildShareBar.ts';
import type { Ports } from './ports.ts';

export interface ClickContext {
  readonly button: ButtonViewModel;
  readonly placement: string;
  readonly anchor?: Element | null;
  preventDefault(): void;
}

export type ClickHandler = (context: ClickContext) => Promise<void>;

export const handleShareClick = (ports: Ports, messages: { readonly copied: string }): ClickHandler =>
  async ({ button, placement, anchor, preventDefault }) => {
    switch (decideClick(button)) {
      case 'copy': {
        preventDefault();
        try {
          await ports.clipboard.write(button.url);
          ports.notifier.notify(messages.copied, anchor);
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
    }
    ports.tracker.track({ service: button.key, url: button.url, placement });
  };
