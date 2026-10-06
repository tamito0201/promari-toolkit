// 描画用の小さな部品。外部由来の文字は textContent / append で扱う。
const SVG_NS = "http://www.w3.org/2000/svg";
export const COLORS = {
  mint: "#9dc1b4",
  teal: "#498e83",
  sky: "#759de0",
  codex: "#c7a4e8",
  codexOther: "#e3be78",
};
export function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  className = "",
  ...children: (Node | string | null)[]
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag);
  node.className = className;
  for (const child of children) if (child !== null) node.append(child);
  return node;
}
export function byId(id: string): HTMLElement {
  const node = document.getElementById(id);
  if (!node) throw new Error(`要素がありません: ${id}`);
  return node;
}
export function svg<K extends keyof SVGElementTagNameMap>(
  tag: K,
  attrs: Record<string, string | number> = {},
  text?: string,
): SVGElementTagNameMap[K] {
  const node = document.createElementNS(SVG_NS, tag);
  for (const [key, value] of Object.entries(attrs))
    node.setAttribute(key, String(value));
  if (text !== undefined) node.textContent = text;
  return node;
}
export function link(
  text: string,
  href: string,
  className = "",
): HTMLAnchorElement {
  const a = el("a", className, text);
  a.href = href;
  return a;
}
