/** 共有先の規則（domain のリポジトリ）に、表示用の情報を結合する。 */
import type { ShareService } from '../domain/model/ShareService.ts';
import type { ServiceSpec } from '../domain/model/types.ts';
import type { ShareServiceRepository } from '../domain/repository/ShareServiceRepository.ts';

export interface ServiceAppearance {
  readonly label: string;
  readonly color: string;
  readonly icon: string;
}
/** Generated input: the URL rule and the display metadata of one service. */
export interface ServiceDefinition extends ServiceSpec, ServiceAppearance {}
export interface DisplayService extends ShareService { readonly appearance: ServiceAppearance }
export interface DisplayCatalog extends ShareServiceRepository {
  resolve(keys: readonly string[]): readonly DisplayService[];
}

export const createDisplayCatalog = (
  repository: ShareServiceRepository,
  appearances: readonly (ServiceAppearance & { readonly key: string })[],
): DisplayCatalog => {
  const byKey = new Map(appearances.map(({ key, label, color, icon }) => [key, Object.freeze({ label, color, icon })] as const));
  const appearance = (key: string): ServiceAppearance => {
    const found = byKey.get(key);
    if (!found) throw new Error(`promari-sns-share: 表示情報がありません: ${key}`);
    return found;
  };
  return Object.freeze({
    has: (key: string) => repository.has(key),
    keys: () => repository.keys(),
    resolve: (keys: readonly string[]) => repository.resolve(keys).map(service =>
      Object.freeze({ ...service, appearance: appearance(service.key) })),
  });
};
