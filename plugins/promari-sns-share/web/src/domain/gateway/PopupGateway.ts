/** A share window. Infrastructure implements it. */
export interface PopupGateway {
  /** Return true if the popup opened; otherwise allow normal link navigation. */
  open(href: string, size: { width: number; height: number }): boolean;
}
