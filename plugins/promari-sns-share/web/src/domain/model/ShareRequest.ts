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
}

export class ShareRequest {
  readonly url: string;
  readonly title: string;
  readonly text: string;
  readonly hashtags: readonly string[];
  readonly via: string;
  readonly site: string;

  private constructor({ url, title, text, hashtags = [], via = '', site = '' }: ShareRequestInput) {
    this.url = url;
    this.title = title;
    this.text = text;
    this.hashtags = Object.freeze(hashtags.map(String).filter(Boolean));
    this.via = via.replace(/^@/, '');
    this.site = site;
    Object.freeze(this);
  }

  static create(input: ShareRequestInput): ShareRequest {
    return new ShareRequest(input);
  }

  /** Hashtags joined for query parameters. */
  get hashtagsCsv(): string {
    return this.hashtags.join(',');
  }

  /** Copy the request with a service-specific URL; the original stays unchanged. */
  withUrl(url: string): ShareRequest {
    return new ShareRequest({ ...this, hashtags: this.hashtags, url });
  }
}
