// 画面の位置は URL のハッシュだけで表す。組み立てと解析をこのモジュールに閉じ、
// 再読み込みとブラウザの戻る操作に対応する。
export type Route =
  | { readonly kind: "overview"; readonly key: "" }
  | { readonly kind: "detail"; readonly key: string }
  | { readonly kind: "category"; readonly key: string }
  | { readonly kind: "metric"; readonly key: string }
  | { readonly kind: "missing"; readonly key: "" };
/** リンク先になれる画面。解析できなかった位置（missing）へのリンクは作らない。 */
export type LinkedRoute = Exclude<Route, { readonly kind: "missing" }>;

const OVERVIEW_HASHES: ReadonlySet<string> = new Set([
  "",
  "#overview",
  "#main",
]);
const KEYED_ROUTE = /^#(?<kind>detail|category|metric)\/(?<key>[^/]+)$/u;
const KEYED_KINDS = ["detail", "category", "metric"] as const;
type KeyedKind = (typeof KEYED_KINDS)[number];

function isKeyedKind(value: string | undefined): value is KeyedKind {
  return KEYED_KINDS.some((kind) => kind === value);
}

export function parseRoute(hash: string): Route {
  if (OVERVIEW_HASHES.has(hash)) return { kind: "overview", key: "" };
  const groups = KEYED_ROUTE.exec(hash)?.groups;
  const kind = groups?.["kind"];
  const key = groups?.["key"];
  if (!isKeyedKind(kind) || key === undefined) return { kind: "missing", key: "" };
  try {
    return { kind, key: decodeURIComponent(key) };
  } catch (error) {
    if (error instanceof URIError) return { kind: "missing", key: "" };
    throw error;
  }
}

export function routeHref(route: LinkedRoute): string {
  if (route.kind === "overview") return "#overview";
  return `#${route.kind}/${encodeURIComponent(route.key)}`;
}
