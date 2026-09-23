/**
 * Implements the domain repository over service specifications. The specifications are
 * generated from the PHP service classes at build time, so no request is sent.
 */
import { ShareService } from '../domain/model/ShareService.ts';
import type { ServiceSpec } from '../domain/model/ShareTypes.ts';
import { UnknownServiceError, type ShareServiceRepository } from '../domain/repository/ShareServiceRepository.ts';

export class SpecShareServiceRepository implements ShareServiceRepository {
  readonly #byKey: ReadonlyMap<string, ShareService>;

  constructor(specs: readonly ServiceSpec[]) {
    this.#byKey = new Map(specs.map((spec) => [spec.key, new ShareService(spec)] as const));
    Object.freeze(this);
  }

  has(key: string): boolean { return this.#byKey.has(key); }
  keys(): readonly string[] { return [...this.#byKey.keys()]; }

  resolve(keys: readonly string[]): readonly ShareService[] {
    return keys.map((key) => {
      const service = this.#byKey.get(key);
      if (!service) throw new UnknownServiceError(key);
      return service;
    });
  }
}
