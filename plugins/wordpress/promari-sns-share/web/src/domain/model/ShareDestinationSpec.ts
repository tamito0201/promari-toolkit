import type { ShareAction } from './ShareAction.ts';

/**
 * Share request fields available to URL templates. `draft` is the destination's own draft text
 * (its `draft` template filled in), so an open destination can prefill a service's editor (ADR-0004).
 */
export type ShareRequestField = 'url' | 'title' | 'text' | 'via' | 'site' | 'hashtagsCsv' | 'draft';

/** How a draft template is filled in: `html` escapes each value for HTML, `text` inserts it as is. */
export type DraftFormat = 'text' | 'html';

/**
 * A destination's draft: how the service writes a link card or embed for the article (ADR-0004).
 * Placeholders are {title}, {url}, {description}, {image}, and {host}; the generator rejects others.
 */
export interface DraftSpec {
  readonly template: string;
  readonly format: DraftFormat;
}

/** The URL rule of one share destination, declared in destinations/*.toml and copied into the generated catalog. */
export interface ShareDestinationSpec {
  readonly key: string;
  readonly action: ShareAction;
  readonly endpoint: string;
  readonly params: Readonly<Record<string, ShareRequestField>>;
  readonly draft?: DraftSpec;
}
