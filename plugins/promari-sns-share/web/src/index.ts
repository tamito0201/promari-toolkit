/**
 * Composition root. Choose the concrete implementations and register the custom element.
 * Dependencies: presentation → application → domain ← infrastructure. Infrastructure
 * implements the repository and gateway interfaces that the domain owns.
 */
import { createDisplayCatalog } from './application/ServiceCatalog.ts';
import { handleShareClick } from './application/HandleShareClick.ts';
import { specShareServiceRepository } from './infrastructure/SpecShareServiceRepository.ts';
import { browserClipboard, customEventPublisher, pageContext, popupWindow, webShare } from './infrastructure/browserGateways.ts';
import { CATALOG } from './generated/catalog.ts';
import { DEFAULTS, VERSION } from './generated/defaults.ts';
import { defineElement } from './presentation/PromariSnsShareElement.ts';

declare global {
  interface Window { PromariSnsShare?: { readonly version: string; readonly services: readonly string[] } }
}

// 四層の外側に置く起動時の組み立て。具体的な接続実装を選ぶ場所はここに限定する。
defineElement({
  catalog: createDisplayCatalog(specShareServiceRepository(CATALOG), CATALOG),
  defaults: DEFAULTS,
  connect: element => ({
    pageContext,
    canNativeShare: () => webShare().available,
    clickHandler: eventName => handleShareClick({
      clipboard: browserClipboard(),
      nativeShare: webShare(),
      popup: popupWindow(),
      activity: customEventPublisher(element, eventName),
    }),
  }),
});
window.PromariSnsShare = Object.freeze({ version: VERSION, services: CATALOG.map((s) => s.key) });
