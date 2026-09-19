/** 生成した入力から、共有先の規則と表示用の情報を分けて組み立てる。 */
import { createCatalog } from '../domain/Service.ts';
import type { Catalog, Service, ServiceSpec } from '../domain/types.ts';

export interface ServiceAppearance {
  readonly label: string;
  readonly color: string;
  readonly icon: string;
}
export interface ServiceDefinition extends ServiceSpec, ServiceAppearance {}
export interface DisplayService extends Service { readonly appearance: ServiceAppearance }
export interface DisplayCatalog extends Catalog {
  resolve(keys: readonly string[]): readonly DisplayService[];
}

export const createDisplayCatalog = (definitions: readonly ServiceDefinition[]): DisplayCatalog => {
  const catalog = createCatalog(definitions.map(({ key, action, endpoint, params }) => ({ key, action, endpoint, params })));
  const appearances = new Map(definitions.map(({ key, label, color, icon }) =>
    [key, Object.freeze({ label, color, icon })] as const));
  return Object.freeze({
    has: (key: string) => catalog.has(key),
    keys: () => catalog.keys(),
    resolve: (keys: readonly string[]) => catalog.resolve(keys).map(service =>
      Object.freeze({ ...service, appearance: appearances.get(service.key)! })),
  });
};
