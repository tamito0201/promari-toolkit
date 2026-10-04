/**
 * Read the element's attributes, the component's public input, into configuration. JSON config overrides individual attributes, which override defaults. Invalid JSON is reported to the console.
 */
import type { DeepPartial, ShareSettings } from '../application/ShareSettings.ts';

const csv = (value: string): string[] => value.split(',').map((s) => s.trim()).filter(Boolean);
const bool = (value: string): boolean => !['false', '0', 'off', 'no'].includes(value.trim().toLowerCase());
const isPlainObject = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);

/**
 * Merge the patch into base. Keys only the patch has are kept too: the maps keyed by destination
 * (labels, buttons) start with only the defaults' keys, and an override for any other destination
 * must not be dropped.
 */
const deepMerge = <T extends object>(base: T, patch: DeepPartial<T> | undefined): T => {
  if (!isPlainObject(patch)) return base;
  const merged: Record<string, unknown> = { ...(base as Record<string, unknown>) };
  for (const [key, next] of Object.entries(patch)) {
    if (next === undefined) continue;
    const value = merged[key];
    merged[key] = isPlainObject(value) && isPlainObject(next) ? deepMerge(value, next) : next;
  }
  return merged as T;
};

type Patch = (value: string) => DeepPartial<ShareSettings>;
type Appearance = ShareSettings['appearance'];

/** An enumerated attribute: a value outside the allowed set keeps the configured value. */
const oneOf = <K extends keyof Appearance>(key: K, allowed: readonly Appearance[K][]): Patch =>
  (v) => ((allowed as readonly string[]).includes(v) ? { appearance: { [key]: v } as DeepPartial<Appearance> } : {});

const ATTRIBUTES: Readonly<Record<string, Patch>> = {
  destinations: (v) => ({ destinations: csv(v) }),
  secondary: (v) => ({ secondary: csv(v) }),
  heading: (v) => ({ heading: v }),
  accent: (v) => ({ style: { accent: v } }),
  size: oneOf('size', ['small', 'large']),
  'label-style': oneOf('label_style', ['icon_text', 'icon', 'text']),
  shape: oneOf('shape', ['official', 'pill', 'rounded', 'square']),
  'heading-position': oneOf('heading_position', ['left', 'top', 'none']),
  hashtags: (v) => ({ text: { hashtags: csv(v) } }),
  via: (v) => ({ text: { via: v } }),
  'title-template': (v) => ({ text: { title_template: v } }),
  utm: (v) => ({ utm: { enabled: bool(v) } }),
  popup: (v) => ({ behavior: { popup: bool(v) } }),
  'new-tab': (v) => ({ behavior: { open_in_new_tab: bool(v) } }),
  // Number('') is 0 and Number('abc') is NaN, which would show the bar at once or never; keep the configured value.
  after: (v) => {
    const after = v.trim() === '' ? NaN : Number(v);
    return Number.isFinite(after) && after >= 0 ? { floating: { after } } : {};
  },
};

const OBSERVED: readonly string[] = Object.freeze([...Object.keys(ATTRIBUTES), 'config', 'url', 'title', 'description', 'image', 'placement']);

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
  /** Observe all configuration attributes, plus URL, title, description, image, and placement. */
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
