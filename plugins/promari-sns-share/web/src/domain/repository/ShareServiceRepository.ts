/**
 * Repository interface owned by the domain. The infrastructure layer implements it,
 * so the dependency points from infrastructure to domain.
 */
import type { ShareService } from '../model/ShareService.ts';

export interface ShareServiceRepository {
  has(key: string): boolean;
  keys(): readonly string[];
  /** Resolve services in the given order. Unknown keys throw rather than being silently skipped. */
  resolve(keys: readonly string[]): readonly ShareService[];
}

export class UnknownServiceError extends Error {
  constructor(key: string) {
    super(`promari-sns-share: 未知のシェア先です: ${key}`);
    this.name = 'UnknownServiceError';
  }
}
