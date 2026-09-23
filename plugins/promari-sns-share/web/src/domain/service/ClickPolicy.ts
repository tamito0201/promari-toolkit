/**
 * Domain service: decide what a click on a share button does.
 */
import { Action } from '../model/ShareTypes.ts';

export type ClickDecision = 'copy' | 'native' | 'popup' | 'follow';

export class ClickPolicy {
  /** 小窓で開ける共有先か。mailto: はメールソフトへ渡すので、窓を開いても何も残らない。 */
  static canOpenInPopup(href: string): boolean {
    return !href.startsWith('mailto:');
  }

  /** Choose the click action; the application layer executes it through gateways. */
  static decide({ action, popup }: { action: Action; popup: unknown }): ClickDecision {
    return action === Action.Copy ? 'copy' : action === Action.Native ? 'native' : popup ? 'popup' : 'follow';
  }
}
