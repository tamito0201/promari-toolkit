/**
 * Domain service: decide what a click on a share button does.
 */
import { ShareAction } from '../model/ShareAction.ts';

export type ShareActionDecision = 'copy' | 'native' | 'popup' | 'follow';

export class ShareActionPolicy {
  /** 小窓で開ける共有先か。mailto: はメールソフトへ渡すので、窓を開いても何も残らない。 */
  static canOpenInPopup(href: string): boolean {
    return !href.startsWith('mailto:');
  }

  /** Choose the click action; the application layer executes it through gateways. */
  static decide({ action, popup }: { action: ShareAction; popup: unknown }): ShareActionDecision {
    return action === ShareAction.Copy ? 'copy' : action === ShareAction.Native ? 'native' : popup ? 'popup' : 'follow';
  }
}
