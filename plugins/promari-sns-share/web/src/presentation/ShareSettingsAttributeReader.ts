/**
 * Read the element's attributes, the component's public input, into configuration. JSON config overrides individual attributes, which override defaults. Invalid JSON is reported to the console.
 */
import type { DeepPartial, ShareSettings } from '../application/ShareSettings.ts';

const csv = (value: string): string[] => value.split(',').map((s) => s.trim()).filter(Boolean);
const bool = (value: string): boolean => !['false', '0', 'off', 'no'].includes(value.trim().toLowerCase());
const isPlainObject = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);

const deepMerge = <T extends object>(base: T, patch: DeepPartial<T> | undefined): T =>
  Object.fromEntries(
    Object.entries(base).map(([key, value]) => {
      const next = (patch as Record<string, unknown> | undefined)?.[key];
      if (next === undefined) return [key, value];
      return [key, isPlainObject(value) && isPlainObject(next) ? deepMerge(value, next) : next];
    }),
  ) as T;

type Patch = (value: string) => DeepPartial<ShareSettings>;

const ATTRIBUTES: Readonly<Record<string, Patch>> = {
  destinations: (v) => ({ destinations: csv(v) }),
  secondary: (v) => ({ secondary: csv(v) }),
  heading: (v) => ({ heading: v }),
  accent: (v) => ({ style: { accent: v } }),
  size: (v) => ({ appearance: { size: v as ShareSettings['appearance']['size'] } }),
  'label-style': (v) => ({ appearance: { label_style: v as ShareSettings['appearance']['label_style'] } }),
  shape: (v) => ({ appearance: { shape: v as ShareSettings['appearance']['shape'] } }),
  'heading-position': (v) => ({ appearance: { heading_position: v as ShareSettings['appearance']['heading_position'] } }),
  hashtags: (v) => ({ text: { hashtags: csv(v) } }),
  via: (v) => ({ text: { via: v } }),
  'title-template': (v) => ({ text: { title_template: v } }),
  utm: (v) => ({ utm: { enabled: bool(v) } }),
  popup: (v) => ({ behavior: { popup: bool(v) } }),
  'new-tab': (v) => ({ behavior: { open_in_new_tab: bool(v) } }),
  after: (v) => ({ floating: { after: Number(v) } }),
};

const OBSERVED: readonly string[] = Object.freeze([...Object.keys(ATTRIBUTES), 'config', 'url', 'title', 'placement']);

const readConfig = (element: Element, defaults: ShareSettings): ShareSettings => {
  const fromAttributes = Object.entries(ATTRIBUTES)
    .filter(([name]) => element.hasAttribute(name))
    .reduce((acc, [name, toPatch]) => deepMerge(acc, toPatch(element.getAttribute(name) ?? '')), defaults);
  const raw = element.getAttribute('config');
  if (!raw) return fromAttributes;
  try {
    return deepMerge(fromAttributes, JSON.parse(raw) as DeepPartial<ShareSettings>);
  } catch (error) {
    console.error('promari-sns-share: config 属性の JSON が読めません', error);
    return fromAttributes;
  }
};

export class ShareSettingsAttributeReader {
  /** Observe all configuration attributes, plus URL, title, and placement. */
  static readonly observedAttributes: readonly string[] = OBSERVED;

  /** Merge nested objects partially; arrays are replaced. */
  static merge<T extends object>(base: T, patch: DeepPartial<T> | undefined): T {
    return deepMerge(base, patch);
  }

  readonly #defaults: ShareSettings;

  constructor(defaults: ShareSettings) {
    this.#defaults = defaults;
  }

  read(element: Element): ShareSettings {
    return readConfig(element, this.#defaults);
  }
}
