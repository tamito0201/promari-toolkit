/**
 * Composition root. Build the catalog and register the custom element. Dependencies point from presentation to application to domain; infrastructure implements application ports.
 */
import { createDisplayCatalog } from './application/ServiceCatalog.ts';
import { OBSERVED_ATTRIBUTES, readConfig } from './infrastructure/AttributeConfig.ts';
import { browserClipboard, customEventTracker, pageContext, popupWindow, scrollWatcher, webShare } from './infrastructure/browserPorts.ts';
import { CATALOG } from './generated/catalog.ts';
import { DEFAULTS, VERSION } from './generated/defaults.ts';
import { defineElement } from './presentation/PromariSnsShareElement.ts';

declare global {
  interface Window { PromariSnsShare?: { readonly version: string; readonly services: readonly string[] } }
}

// 四層の外側に置く起動時の組み立て。具体的な接続実装を選ぶ場所はここに限定する。
defineElement({
  catalog: createDisplayCatalog(CATALOG),
  observedAttributes: OBSERVED_ATTRIBUTES,
  connect: element => ({
    readConfig: () => readConfig(element, DEFAULTS),
    pageContext,
    canNativeShare: () => webShare().available,
    ports: (notifier, eventName) => ({
      clipboard: browserClipboard(), sharer: webShare(), popup: popupWindow(),
      notifier, tracker: customEventTracker(element, eventName),
    }),
    watchScroll: config => scrollWatcher(element, config),
  }),
});
window.PromariSnsShare = Object.freeze({ version: VERSION, services: CATALOG.map((s) => s.key) });
