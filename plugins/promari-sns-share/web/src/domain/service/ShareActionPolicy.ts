/**
 * Domain service: decide what a click on a share button does.
 */
import { ShareAction } from '../model/ShareAction.ts';

export type ShareActionDecision = 'copy' | 'native' | 'compose' | 'popup' | 'follow';

export class ShareActionPolicy {
  /** 小窓で開ける共有先か。mailto: はメールソフトへ渡すので、窓を開いても何も残らない。 */
  static canOpenInPopup(href: string): boolean {
    return !href.startsWith('mailto:');
  }

  /** Choose the click action; the application layer executes it through gateways. */
  static decide({ action, popup }: { action: ShareAction; popup: unknown }): ShareActionDecision {
    switch (action) {
      case ShareAction.Copy: return 'copy';
      case ShareAction.Native: return 'native';
      case ShareAction.Compose: return 'compose';
      case ShareAction.Open: return popup ? 'popup' : 'follow';
    }
  }

  /**
   * The text a compose destination puts on the clipboard: the page title, a line break, and the
   * shared URL. Without a title only the URL is copied, so the paste never starts with a blank line.
   */
  static composeDraft(title: string, url: string): string {
    const heading = title.trim();
    return heading ? `${heading}\n${url}` : url;
  }
}
