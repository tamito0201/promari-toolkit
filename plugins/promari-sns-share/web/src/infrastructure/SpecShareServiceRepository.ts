/**
 * Implements the domain repository over service specifications. The specifications are
 * generated from the PHP service classes at build time, so no request is sent.
 */
import { createShareService, type ShareService } from '../domain/model/ShareService.ts';
import type { ServiceSpec } from '../domain/model/types.ts';
import { UnknownServiceError, type ShareServiceRepository } from '../domain/repository/ShareServiceRepository.ts';

export const specShareServiceRepository = (specs: readonly ServiceSpec[]): ShareServiceRepository => {
  const byKey = new Map(specs.map((spec) => [spec.key, createShareService(spec)] as const));
  const get = (key: string): ShareService => {
    const service = byKey.get(key);
    if (!service) throw new UnknownServiceError(key);
    return service;
  };
  return Object.freeze({
    has: (key: string) => byKey.has(key),
    keys: () => [...byKey.keys()],
    resolve: (keys: readonly string[]) => keys.map(get),
  });
};
