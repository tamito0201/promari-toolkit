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
}

/** Generated input: the URL rule and the display metadata of one destination. */
export interface ShareDestinationDefinition extends ShareDestinationSpec, DestinationAppearance {}

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
  shareUrl(request: ShareRequest): string { return this.#destination.shareUrl(request); }
}

export class ShareButtonCatalog {
  readonly #repository: ShareDestinationRepository;
  readonly #appearances: ReadonlyMap<string, DestinationAppearance>;

  constructor(repository: ShareDestinationRepository, appearances: readonly (DestinationAppearance & { readonly key: string })[]) {
    this.#repository = repository;
    this.#appearances = new Map(appearances.map(({ key, label, color, icon }) => [key, Object.freeze({ label, color, icon })] as const));
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
