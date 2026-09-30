/**
 * Immutable value object describing the shared content. URLs already include UTM
 * parameters and text is already formatted.
 */
export interface ShareRequestInput {
  readonly url: string;
  readonly title: string;
  readonly text: string;
  readonly hashtags?: readonly string[];
  readonly via?: string;
  readonly site?: string;
  /** The page description, used only by link-card drafts (ADR-0004). */
  readonly description?: string;
  /** The page image URL, used only by link-card drafts (ADR-0004). */
  readonly image?: string;
}

/** The values a link card may shorten or leave out to fit a URL length limit. */
export type ShareRequestPreview = Partial<Pick<ShareRequestInput, 'title' | 'description' | 'image'>>;

export class ShareRequest {
  readonly url: string;
  readonly title: string;
  readonly text: string;
  readonly hashtags: readonly string[];
  readonly via: string;
  readonly site: string;
  readonly description: string;
  readonly image: string;

  private constructor({ url, title, text, hashtags = [], via = '', site = '', description = '', image = '' }: ShareRequestInput) {
    this.url = url;
    this.title = title;
    this.text = text;
    this.hashtags = Object.freeze(hashtags.map(String).filter(Boolean));
    this.via = via.replace(/^@/, '');
    this.site = site;
    this.description = description;
    this.image = image;
    Object.freeze(this);
  }

  static create(input: ShareRequestInput): ShareRequest {
    return new ShareRequest(input);
  }

  /** Hashtags joined for query parameters. */
  get hashtagsCsv(): string {
    return this.hashtags.join(',');
  }

  /** Copy the request with a destination-specific URL; the original stays unchanged. */
  withUrl(url: string): ShareRequest {
    return new ShareRequest({ ...this, hashtags: this.hashtags, url });
  }

  /** Copy the request with a shortened or removed title, description, or image; the original stays unchanged. */
  withPreview(preview: ShareRequestPreview): ShareRequest {
    return new ShareRequest({ ...this, hashtags: this.hashtags, ...preview });
  }
}
