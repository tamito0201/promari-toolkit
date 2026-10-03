/**
 * Repository interface owned by the domain. The infrastructure layer implements it,
 * so the dependency points from infrastructure to domain.
 */
import type { ShareDestination } from '../model/ShareDestination.ts';

export interface ShareDestinationRepository {
  has(key: string): boolean;
  keys(): readonly string[];
  /** Resolve destinations in the given order. Unknown keys throw rather than being silently skipped. */
  resolve(keys: readonly string[]): readonly ShareDestination[];
}

export class UnknownShareDestinationError extends Error {
  constructor(key: string) {
    super(`promari-sns-share: 未知のシェア先です: ${key}`);
    this.name = 'UnknownShareDestinationError';
  }
}
