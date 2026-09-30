/** A share window. Infrastructure implements it. */
export interface ShareWindowGateway {
  /** Return true if the popup opened; otherwise allow normal link navigation. */
  open(href: string, size: { width: number; height: number }): boolean;
  /**
   * Open a page in a new tab, detached from this page (no opener, no referrer). Call it synchronously
   * inside the click so popup blockers treat it as user-initiated. Whether the tab opened cannot be
   * observed: browsers return no window handle for a detached tab.
   */
  openTab(href: string): void;
}
