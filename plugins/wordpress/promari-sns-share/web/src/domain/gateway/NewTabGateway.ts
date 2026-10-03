/**
 * A new browser tab. Infrastructure implements it. It is separate from ShareWindowGateway: a popup
 * reports whether it opened and falls back to the link, while a new tab cannot be observed at all.
 */
export interface NewTabGateway {
  /**
   * Open a page in a new tab, detached from this page (no opener, no referrer). Call it synchronously
   * inside the click so popup blockers treat it as user-initiated. Whether the tab opened cannot be
   * observed: browsers return no window handle for a detached tab.
   */
  open(href: string): void;
}
