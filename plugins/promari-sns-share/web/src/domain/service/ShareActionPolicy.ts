/**
 * Domain service: decide what a click on a share button does, and what that means for the control.
 */
import { ShareAction } from '../model/ShareAction.ts';

export type ShareActionDecision = 'copy' | 'native' | 'compose' | 'popup' | 'follow';

/**
 * How each action relates to the button's href. A Record over ShareAction, so adding an action
 * does not compile until this table states both facts for it.
 * - followsLink: the action is following the href (the share URL).
 * - linkable: the control may be a link. Following the href without the click handler must not lose
 *   the action: copy and native point at the page itself, but compose would open the editor
 *   without the copied title and URL (ADR-0003).
 */
const CONTROL: Readonly<Record<ShareAction, { readonly followsLink: boolean; readonly linkable: boolean }>> = {
  [ShareAction.Open]: { followsLink: true, linkable: true },
  [ShareAction.Copy]: { followsLink: false, linkable: true },
  [ShareAction.Native]: { followsLink: false, linkable: true },
  [ShareAction.Compose]: { followsLink: false, linkable: false },
};

export class ShareActionPolicy {
  /** 小窓で開ける共有先か。mailto: はメールソフトへ渡すので、窓を開いても何も残らない。 */
  static canOpenInPopup(href: string): boolean {
    return !href.startsWith('mailto:');
  }

  /** Whether the action is following the share URL (open). Link attributes such as rel apply only then. */
  static followsLink(action: ShareAction): boolean {
    return CONTROL[action].followsLink;
  }

  /** Whether the control may be rendered as a link to its href (false for compose). */
  static linkable(action: ShareAction): boolean {
    return CONTROL[action].linkable;
  }

  /** Choose the click action; the application layer executes it through gateways. */
  static decide({ action, popup }: { action: ShareAction; popup: unknown }): ShareActionDecision {
    switch (action) {
      case ShareAction.Copy: return 'copy';
      case ShareAction.Native: return 'native';
      case ShareAction.Compose: return 'compose';
      case ShareAction.Open: return popup ? 'popup' : 'follow';
      default: {
        // ShareAction を増やしたらここでコンパイルが止まる。
        const unhandled: never = action;
        throw new Error(`promari-sns-share: 未対応の操作です: ${String(unhandled)}`);
      }
    }
  }
}
