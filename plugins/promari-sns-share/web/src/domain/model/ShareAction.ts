/** What a click on a share button does. Matches the PHP ShareAction enum. */
export const ShareAction = { Open: 'open', Copy: 'copy', Native: 'native' } as const;
export type ShareAction = (typeof ShareAction)[keyof typeof ShareAction];
