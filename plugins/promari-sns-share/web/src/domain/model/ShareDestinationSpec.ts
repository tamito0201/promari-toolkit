import type { ShareAction } from './ShareAction.ts';

/** Share request fields available to URL templates. */
export type ShareRequestField = 'url' | 'title' | 'text' | 'via' | 'site' | 'hashtagsCsv';

/** The URL rule of one share destination, extracted from PHP into the generated catalog. */
export interface ShareDestinationSpec {
  readonly key: string;
  readonly action: ShareAction;
  readonly endpoint: string;
  readonly params: Readonly<Record<string, ShareRequestField>>;
}
