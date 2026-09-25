/** What a click on a share button does. Destination files (destinations/*.toml) use the same values. */
export const ShareAction = { Open: 'open', Copy: 'copy', Native: 'native' } as const;
export type ShareAction = (typeof ShareAction)[keyof typeof ShareAction];
