/**
 * Domain service: expand the shared text template.
 */
import type { SharedPage } from '../model/SharedPage.ts';

export class ShareTextFormatter {
  /**
   * Expand the title, site, and URL placeholders in a single pass.
   * 置換した結果は走査し直さない。題名に {site} が含まれても、それは題名のまま残る。
   */
  static format(template: string, { url, title, site }: SharedPage): string {
    const values: Readonly<Record<string, string>> = { '{title}': title, '{site}': site, '{url}': url };
    return template.replace(/\{(?:title|site|url)\}/g, (placeholder) => values[placeholder] ?? placeholder);
  }
}
