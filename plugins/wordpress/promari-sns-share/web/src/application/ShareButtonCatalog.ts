/** 共有先の規則（domain のリポジトリ）に、表示用の情報を結合する。 */
import type { ShareRequest } from '../domain/model/ShareRequest.ts';
import type { ShareDestination } from '../domain/model/ShareDestination.ts';
import type { ShareAction } from '../domain/model/ShareAction.ts';
import type { ShareDestinationSpec } from '../domain/model/ShareDestinationSpec.ts';
import type { ShareDestinationRepository } from '../domain/repository/ShareDestinationRepository.ts';

export interface DestinationAppearance {
  readonly label: string;
  readonly color: string;
  readonly icon: string;
  /** Shown after a compose destination's success notice, such as how to turn the paste into a card. Empty for none. */
  readonly composeHint: string;
}

/** Display metadata as generated: the compose hint is present only for destinations that declare one. */
export type DestinationAppearanceInput = Omit<DestinationAppearance, 'composeHint'> & { readonly composeHint?: string };

/** Generated input: the URL rule and the display metadata of one destination. */
export type ShareDestinationDefinition = ShareDestinationSpec & DestinationAppearanceInput;

/** A domain share destination together with how it is shown. */
export class DisplayedShareDestination {
  readonly appearance: DestinationAppearance;
  readonly #destination: ShareDestination;

  constructor(destination: ShareDestination, appearance: DestinationAppearance) {
    this.#destination = destination;
    this.appearance = appearance;
    Object.freeze(this);
  }

  get key(): string { return this.#destination.key; }
  get action(): ShareAction { return this.#destination.action; }
  get sendsDraft(): boolean { return this.#destination.sendsDraft; }
  shareUrl(request: ShareRequest): string { return this.#destination.shareUrl(request); }
  composeDraft(request: ShareRequest): string { return this.#destination.composeDraft(request); }
}

export class ShareButtonCatalog {
  readonly #repository: ShareDestinationRepository;
  readonly #appearances: ReadonlyMap<string, DestinationAppearance>;

  constructor(repository: ShareDestinationRepository, appearances: readonly (DestinationAppearanceInput & { readonly key: string })[]) {
    this.#repository = repository;
    this.#appearances = new Map(appearances.map(({ key, label, color, icon, composeHint = '' }) => [key, Object.freeze({ label, color, icon, composeHint })] as const));
    Object.freeze(this);
  }

  /** The domain repository behind this catalog, for domain policies. */
  get repository(): ShareDestinationRepository { return this.#repository; }

  has(key: string): boolean { return this.#repository.has(key); }
  keys(): readonly string[] { return this.#repository.keys(); }

  resolve(keys: readonly string[]): readonly DisplayedShareDestination[] {
    return this.#repository.resolve(keys).map((destination) => new DisplayedShareDestination(destination, this.#appearance(destination.key)));
  }

  #appearance(key: string): DestinationAppearance {
    const found = this.#appearances.get(key);
    if (!found) throw new Error(`promari-sns-share: 表示情報がありません: ${key}`);
    return found;
  }
}
