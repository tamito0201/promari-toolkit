/**
 * Implements the domain repository over destination specifications. The specifications are
 * generated from destinations/*.toml at build time, so no request is sent.
 */
import { ShareDestination } from '../domain/model/ShareDestination.ts';
import type { ShareDestinationSpec } from '../domain/model/ShareDestinationSpec.ts';
import { UnknownShareDestinationError, type ShareDestinationRepository } from '../domain/repository/ShareDestinationRepository.ts';

export class InMemoryShareDestinationRepository implements ShareDestinationRepository {
  readonly #byKey: ReadonlyMap<string, ShareDestination>;

  constructor(specs: readonly ShareDestinationSpec[]) {
    this.#byKey = new Map(specs.map((spec) => [spec.key, new ShareDestination(spec)] as const));
    Object.freeze(this);
  }

  has(key: string): boolean { return this.#byKey.has(key); }
  keys(): readonly string[] { return [...this.#byKey.keys()]; }

  resolve(keys: readonly string[]): readonly ShareDestination[] {
    return keys.map((key) => {
      const destination = this.#byKey.get(key);
      if (!destination) throw new UnknownShareDestinationError(key);
      return destination;
    });
  }
}
