/**
 * What a click on a share button does. Destination files (destinations/*.toml) use the same values.
 * - open: follow the destination's share URL
 * - copy: put the page URL on the clipboard
 * - native: open the device share sheet
 * - compose: copy the title and URL, then open the destination's editor in a new tab (ADR-0003)
 */
export const ShareAction = { Open: 'open', Copy: 'copy', Native: 'native', Compose: 'compose' } as const;
export type ShareAction = (typeof ShareAction)[keyof typeof ShareAction];
