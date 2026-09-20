/**
 * RFC 3986 のパーセント符号化。PHP 側の rawurlencode と同じ結果を返す。
 */

/**
 * encodeURIComponent は ! ' ( ) * を符号化せずに残すため、そのままでは
 * rawurlencode を使う PHP 側と URL が食い違う。この5文字を補って揃える。
 */
export const encodeComponent = (value: string): string =>
  encodeURIComponent(value).replace(/[!'()*]/g, (char) => `%${char.charCodeAt(0).toString(16).toUpperCase()}`);
