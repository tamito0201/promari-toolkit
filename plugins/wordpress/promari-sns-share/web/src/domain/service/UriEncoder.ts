/**
 * Domain service: RFC 3986 percent-encoding and query appending.
 */
export class UriEncoder {
  /**
   * encodeURIComponent は ! ' ( ) * を符号化せずに残す。これらは RFC 3986 では
   * 予約文字なので、共有先が文字どおりに解釈できるよう補って符号化する。
   */
  static encode(value: string): string {
    return encodeURIComponent(value).replace(/[!'()*]/g, (char) => `%${char.charCodeAt(0).toString(16).toUpperCase()}`);
  }

  /** Append query parameters while preserving fragments. */
  static appendQuery(url: string, params: Readonly<Record<string, string>>): string {
    const hash = url.indexOf('#');
    const [base, fragment] = hash >= 0 ? [url.slice(0, hash), url.slice(hash + 1)] : [url, undefined];
    const query = Object.entries(params).map(([k, v]) => `${UriEncoder.encode(k)}=${UriEncoder.encode(v)}`).join('&');
    return `${base}${base.includes('?') ? '&' : '?'}${query}${fragment === undefined ? '' : `#${fragment}`}`;
  }
}
