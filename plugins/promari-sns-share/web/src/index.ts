/**
 * Composition root. Build the catalog and register the custom element. Dependencies point from presentation to application to domain; infrastructure implements application ports.
 */
import { createCatalog } from './domain/Service.ts';
import { CATALOG } from './generated/catalog.ts';
import { DEFAULTS, VERSION } from './generated/defaults.ts';
import { defineElement } from './presentation/PromariSnsShareElement.ts';

declare global {
  interface Window { PromariSnsShare?: { readonly version: string; readonly services: readonly string[] } }
}

defineElement({ catalog: createCatalog(CATALOG), defaults: DEFAULTS });
window.PromariSnsShare = Object.freeze({ version: VERSION, services: CATALOG.map((s) => s.key) });
