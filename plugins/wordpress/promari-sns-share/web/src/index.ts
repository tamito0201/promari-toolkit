/**
 * Entry point. Build the DI container and register the custom element.
 * Dependencies: presentation → application → domain ← infrastructure. Infrastructure
 * implements the repository and gateway interfaces that the domain owns; composition/
 * binds them together with InversifyJS.
 */
import { ShareContainer } from './composition/ShareContainer.ts';
import { TOKENS } from './composition/InjectionTokens.ts';
import { CATALOG } from './generated/catalog.ts';
import { DEFAULTS, VERSION } from './generated/defaults.ts';
import { PromariSnsShareElement } from './presentation/PromariSnsShareElement.ts';

declare global {
  interface Window { PromariSnsShare?: { readonly version: string; readonly destinations: readonly string[] } }
}

const container = ShareContainer.create(CATALOG, DEFAULTS);
PromariSnsShareElement.define(ShareContainer.get(container, TOKENS.ShareElementDependencies));
window.PromariSnsShare = Object.freeze({ version: VERSION, destinations: CATALOG.map((s) => s.key) });
