/** 共有先の規則（domain のリポジトリ）に、表示用の情報を結合する。 */
import type { ShareRequest } from '../domain/model/ShareRequest.ts';
import type { ShareService } from '../domain/model/ShareService.ts';
import type { Action, ServiceSpec } from '../domain/model/ShareTypes.ts';
import type { ShareServiceRepository } from '../domain/repository/ShareServiceRepository.ts';

export interface ServiceAppearance {
  readonly label: string;
  readonly color: string;
  readonly icon: string;
}

/** Generated input: the URL rule and the display metadata of one service. */
export interface ServiceDefinition extends ServiceSpec, ServiceAppearance {}

/** A domain share service together with how it is shown. */
export class DisplayService {
  readonly appearance: ServiceAppearance;
  readonly #service: ShareService;

  constructor(service: ShareService, appearance: ServiceAppearance) {
    this.#service = service;
    this.appearance = appearance;
    Object.freeze(this);
  }

  get key(): string { return this.#service.key; }
  get action(): Action { return this.#service.action; }
  shareUrl(request: ShareRequest): string { return this.#service.shareUrl(request); }
}

export class DisplayCatalog {
  readonly #repository: ShareServiceRepository;
  readonly #appearances: ReadonlyMap<string, ServiceAppearance>;

  constructor(repository: ShareServiceRepository, appearances: readonly (ServiceAppearance & { readonly key: string })[]) {
    this.#repository = repository;
    this.#appearances = new Map(appearances.map(({ key, label, color, icon }) => [key, Object.freeze({ label, color, icon })] as const));
    Object.freeze(this);
  }

  /** The domain repository behind this catalog, for domain policies. */
  get repository(): ShareServiceRepository { return this.#repository; }

  has(key: string): boolean { return this.#repository.has(key); }
  keys(): readonly string[] { return this.#repository.keys(); }

  resolve(keys: readonly string[]): readonly DisplayService[] {
    return this.#repository.resolve(keys).map((service) => new DisplayService(service, this.#appearance(service.key)));
  }

  #appearance(key: string): ServiceAppearance {
    const found = this.#appearances.get(key);
    if (!found) throw new Error(`promari-sns-share: 表示情報がありません: ${key}`);
    return found;
  }
}
