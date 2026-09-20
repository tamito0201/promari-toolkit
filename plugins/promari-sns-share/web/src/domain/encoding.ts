/**
 * RFC 3986 のパーセント符号化。
 */

/**
 * encodeURIComponent は ! ' ( ) * を符号化せずに残す。これらは RFC 3986 では
 * 予約文字なので、共有先が文字どおりに解釈できるよう補って符号化する。
 */
export const encodeComponent = (value: string): string =>
  encodeURIComponent(value).replace(/[!'()*]/g, (char) => `%${char.charCodeAt(0).toString(16).toUpperCase()}`);
