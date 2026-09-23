/** HTML escaping for the markup the component renders. */
const ESCAPES: Readonly<Record<string, string>> = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' };

export class HtmlEscaper {
  static escape(value: string): string {
    return value.replace(/[&<>"]/g, (c) => ESCAPES[c] ?? c);
  }
}
