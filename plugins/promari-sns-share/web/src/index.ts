/**
 * Composition root. Choose the concrete implementations and register the custom element.
 * Dependencies: presentation → application → domain ← infrastructure. Infrastructure
 * implements the repository and gateway interfaces that the domain owns.
 */
import { BuildShareBarUseCase } from './application/BuildShareBarUseCase.ts';
import { DisplayCatalog } from './application/DisplayCatalog.ts';
import { HandleShareClickUseCase } from './application/HandleShareClickUseCase.ts';
import { BrowserClipboardGateway } from './infrastructure/BrowserClipboardGateway.ts';
import { BrowserPageContext } from './infrastructure/BrowserPageContext.ts';
import { CustomEventActivityPublisher } from './infrastructure/CustomEventActivityPublisher.ts';
import { PopupWindowGateway } from './infrastructure/PopupWindowGateway.ts';
import { SpecShareServiceRepository } from './infrastructure/SpecShareServiceRepository.ts';
import { WebShareGateway } from './infrastructure/WebShareGateway.ts';
import { CATALOG } from './generated/catalog.ts';
import { DEFAULTS, VERSION } from './generated/defaults.ts';
import { PromariSnsShareElement } from './presentation/PromariSnsShareElement.ts';

declare global {
  interface Window { PromariSnsShare?: { readonly version: string; readonly services: readonly string[] } }
}

// 四層の外側に置く起動時の組み立て。具体的な接続実装を選ぶ場所はここに限定する。
const catalog = new DisplayCatalog(new SpecShareServiceRepository(CATALOG), CATALOG);
const pageContext = new BrowserPageContext();
const webShare = new WebShareGateway();
PromariSnsShareElement.define({
  buildShareBar: new BuildShareBarUseCase(catalog),
  defaults: DEFAULTS,
  connect: element => ({
    pageContext: () => pageContext.read(),
    canNativeShare: () => webShare.available,
    clickUseCase: eventName => new HandleShareClickUseCase({
      clipboard: new BrowserClipboardGateway(),
      nativeShare: webShare,
      popup: new PopupWindowGateway(),
      activity: new CustomEventActivityPublisher(element, eventName),
    }),
  }),
});
window.PromariSnsShare = Object.freeze({ version: VERSION, services: CATALOG.map((s) => s.key) });
