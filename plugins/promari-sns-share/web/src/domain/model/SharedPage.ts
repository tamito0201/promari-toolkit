/**
 * The page being shared. `description` and `image` feed link-card drafts (ADR-0004); either is
 * empty when the page does not declare it.
 */
export interface SharedPage {
  readonly url: string;
  readonly title: string;
  readonly site: string;
  readonly description: string;
  readonly image: string;
}
