/**
 * Read the element's attributes, the component's public input, into configuration. JSON config overrides individual attributes, which override defaults. Invalid JSON is reported to the console.
 */
import type { DeepPartial, ShareConfig } from '../application/config.ts';

const csv = (value: string): string[] => value.split(',').map((s) => s.trim()).filter(Boolean);
const bool = (value: string): boolean => !['false', '0', 'off', 'no'].includes(value.trim().toLowerCase());
const isPlainObject = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);

export const deepMerge = <T extends object>(base: T, patch: DeepPartial<T> | undefined): T =>
  Object.fromEntries(
    Object.entries(base).map(([key, value]) => {
      const next = (patch as Record<string, unknown> | undefined)?.[key];
      if (next === undefined) return [key, value];
      return [key, isPlainObject(value) && isPlainObject(next) ? deepMerge(value, next) : next];
    }),
  ) as T;

type Patch = (value: string) => DeepPartial<ShareConfig>;

const ATTRIBUTES: Readonly<Record<string, Patch>> = {
  services: (v) => ({ services: csv(v) }),
  secondary: (v) => ({ secondary: csv(v) }),
  heading: (v) => ({ heading: v }),
  accent: (v) => ({ style: { accent: v } }),
  size: (v) => ({ appearance: { size: v as ShareConfig['appearance']['size'] } }),
  'label-style': (v) => ({ appearance: { label_style: v as ShareConfig['appearance']['label_style'] } }),
  shape: (v) => ({ appearance: { shape: v as ShareConfig['appearance']['shape'] } }),
  'heading-position': (v) => ({ appearance: { heading_position: v as ShareConfig['appearance']['heading_position'] } }),
  hashtags: (v) => ({ text: { hashtags: csv(v) } }),
  via: (v) => ({ text: { via: v } }),
  'title-template': (v) => ({ text: { title_template: v } }),
  utm: (v) => ({ utm: { enabled: bool(v) } }),
  popup: (v) => ({ behavior: { popup: bool(v) } }),
  'new-tab': (v) => ({ behavior: { open_in_new_tab: bool(v) } }),
  after: (v) => ({ floating: { after: Number(v) } }),
};

/**
 * Observe all configuration attributes, plus URL, title, and placement.
 */
export const OBSERVED_ATTRIBUTES: readonly string[] = Object.freeze([...Object.keys(ATTRIBUTES), 'config', 'url', 'title', 'placement']);

export const readConfig = (element: Element, defaults: ShareConfig): ShareConfig => {
  const fromAttributes = Object.entries(ATTRIBUTES)
    .filter(([name]) => element.hasAttribute(name))
    .reduce((acc, [name, toPatch]) => deepMerge(acc, toPatch(element.getAttribute(name) ?? '')), defaults);
  const raw = element.getAttribute('config');
  if (!raw) return fromAttributes;
  try {
    return deepMerge(fromAttributes, JSON.parse(raw) as DeepPartial<ShareConfig>);
  } catch (error) {
    console.error('promari-sns-share: config 属性の JSON が読めません', error);
    return fromAttributes;
  }
};
