/**
 * Composition root. Choose the concrete implementations and register the custom element.
 * Dependencies: presentation → application → domain ← infrastructure. Infrastructure
 * implements the repository and gateway interfaces that the domain owns.
 */
import { BuildShareBarUseCase } from './application/BuildShareBarUseCase.ts';
import { ShareButtonCatalog } from './application/ShareButtonCatalog.ts';
import { HandleShareClickUseCase } from './application/HandleShareClickUseCase.ts';
import { BrowserClipboard } from './infrastructure/BrowserClipboard.ts';
import { BrowserSharedPage } from './infrastructure/BrowserSharedPage.ts';
import { CustomEventShareActivityPublisher } from './infrastructure/CustomEventShareActivityPublisher.ts';
import { BrowserPopupWindow } from './infrastructure/BrowserPopupWindow.ts';
import { InMemoryShareDestinationRepository } from './infrastructure/InMemoryShareDestinationRepository.ts';
import { BrowserNativeShare } from './infrastructure/BrowserNativeShare.ts';
import { CATALOG } from './generated/catalog.ts';
import { DEFAULTS, VERSION } from './generated/defaults.ts';
import { PromariSnsShareElement } from './presentation/PromariSnsShareElement.ts';

declare global {
  interface Window { PromariSnsShare?: { readonly version: string; readonly destinations: readonly string[] } }
}

// 四層の外側に置く起動時の組み立て。具体的な接続実装を選ぶ場所はここに限定する。
const catalog = new ShareButtonCatalog(new InMemoryShareDestinationRepository(CATALOG), CATALOG);
const pageContext = new BrowserSharedPage();
const webShare = new BrowserNativeShare();
PromariSnsShareElement.define({
  buildShareBar: new BuildShareBarUseCase(catalog),
  defaults: DEFAULTS,
  connect: element => ({
    pageContext: () => pageContext.read(),
    canNativeShare: () => webShare.available,
    clickUseCase: eventName => new HandleShareClickUseCase({
      clipboard: new BrowserClipboard(),
      nativeShare: webShare,
      popup: new BrowserPopupWindow(),
      activity: new CustomEventShareActivityPublisher(element, eventName),
    }),
  }),
});
window.PromariSnsShare = Object.freeze({ version: VERSION, destinations: CATALOG.map((s) => s.key) });
