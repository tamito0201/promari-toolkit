/** Circular sharing and reaction controls. The host owns reaction persistence. */
import type { ShareBarViewModel, ShareButtonViewModel } from '../application/BuildShareBarUseCase.ts';
import { HtmlEscaper } from './HtmlEscaper.ts';
const icons: Readonly<Record<string, string>> = {"copy": "<g fill=\"none\" stroke=\"currentColor\" stroke-width=\"1.7\" stroke-linecap=\"round\"><rect x=\"8\" y=\"8\" width=\"12\" height=\"13\" rx=\"2\"/><path d=\"M15 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h3\"/></g>", "more": "<circle cx=\"5\" cy=\"12\" r=\"1.8\"/><circle cx=\"12\" cy=\"12\" r=\"1.8\"/><circle cx=\"19\" cy=\"12\" r=\"1.8\"/>", "x": "<path d=\"M18.9 2H22l-6.8 7.8L23.2 22h-6.3L12 14.6 5.5 22H2.3l8.2-9.4L.8 2h6.5l5.8 7.1L18.9 2Zm-1.1 18h1.7L6.4 3.9H4.6L17.8 20Z\"/>", "line": "<path d=\"M22 10.5C22 5.8 17.5 2 12 2S2 5.8 2 10.5c0 4.2 3.6 7.7 8.5 8.4.3.1.6.2.7.4l.1.8-.2 1.2c0 .3-.2 1 .8.6 1-.4 5.4-3.2 7.4-5.6 1.8-1.8 2.7-3.7 2.7-5.8Z\"/><text x=\"12\" y=\"12.7\" text-anchor=\"middle\" fill=\"white\" font-size=\"6.2\" font-family=\"Arial,sans-serif\" font-weight=\"bold\">LINE</text>", "facebook": "<path d=\"M14 22v-9h3l.5-3H14V8c0-.9.3-1.5 1.6-1.5H18V3.8c-.4-.1-1.8-.2-3.2-.2-3.1 0-5.1 1.8-5.1 5.2V10H7v3h2.7v9H14Z\"/>", "native": "<g fill=\"none\" stroke=\"currentColor\" stroke-width=\"1.7\" stroke-linecap=\"round\" stroke-linejoin=\"round\"><path d=\"M12 15V2m-4 4 4-4 4 4M7 9H4v12h16V9h-3\"/></g>"};
const svg = (key: string, fallback = ''): string => icons[key] ? `<svg viewBox="0 0 24 24" aria-hidden="true">${icons[key]}</svg>` : fallback;
const labels: Readonly<Record<string, string>> = { x: 'X', line: 'LINE', facebook: 'Facebook', copy: 'コピー', native: '共有' };
const icon = (key: string, fallback = ''): string => `<span class="circle-icon">${svg(key, fallback)}</span>`;
const button = (b: ShareButtonViewModel, attribute: string): string => {
  const name = HtmlEscaper.escape(labels[b.key] ?? b.label);
  const content = `${icon(b.key, b.icon)}<span class="caption">${name}</span>`;
  const attrs = `class="social" data-key="${HtmlEscaper.escape(b.key)}" ${attribute}="${HtmlEscaper.escape(b.key)}" aria-label="${HtmlEscaper.escape(b.tooltip)}"`;
  return b.action === 'copy' || b.action === 'native'
    ? `<button type="button" ${attrs}>${content}</button>`
    : `<a ${attrs} href="${HtmlEscaper.escape(b.href)}"${b.newTab ? ' target="_blank"' : ''} rel="noopener noreferrer${b.nofollow ? ' nofollow' : ''}">${content}</a>`;
};
const render = (vm: ShareBarViewModel, attribute: string, caption: string, like: boolean): string => {
  const more = vm.secondary.length ? `<details><summary aria-label="その他の共有先"><span class="circle-icon">${svg('more')}</span><span class="caption">その他</span></summary><div class="options"><strong>SHARE THIS STORY</strong>${vm.secondary.map(b => button(b, attribute)).join('')}</div></details>` : '';
  const reaction = like ? `<button class="like" type="button" aria-pressed="false" disabled><span class="heart"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.6a5.5 5.5 0 0 0 0-7.8Z"/></svg></span><span class="like-label">いいね</span><span class="count" aria-label="いいねの件数は未取得">—</span></button>` : '';
  return `<style>${circleCss}</style><div class="circle ${like ? 'with-like' : ''}" role="group" aria-label="${HtmlEscaper.escape(vm.groupLabel)}">${caption ? `<div class="intro"><small>PASS IT ON</small><span>${HtmlEscaper.escape(caption)}</span></div>` : ''}<div class="row">${reaction}<div class="socials">${vm.primary.map(b => button(b, attribute)).join('')}${more}</div></div><p class="status" role="status" aria-live="polite"></p></div>`;
};
const circleCss = `
:host{display:block;color:#292232;font-family:Helvetica,Arial,"Hiragino Sans","Noto Sans JP",sans-serif;container-type:inline-size}
*{box-sizing:border-box}button,a,summary{-webkit-tap-highlight-color:transparent}button{font:inherit;cursor:pointer}a{text-decoration:none}button:disabled{opacity:.5;cursor:wait}
.circle{display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:20px;padding:24px 0;border-block:1px solid #d5ccdf}
.intro{display:grid;gap:8px;flex:1;min-width:170px}.intro small{font-size:10px;font-weight:800;letter-spacing:.2em;color:#745891}.intro>span{font-size:14px;font-weight:700;line-height:1.7;color:#302638}
.row{display:flex;align-items:center;gap:24px;min-width:0}.socials{display:flex;align-items:flex-start;gap:9px}.with-like .socials{padding-left:24px;border-left:1px solid #d5ccdf}
.social,summary{display:flex;flex-direction:column;align-items:center;gap:7px;min-width:44px;padding:0;border:0;background:transparent;color:#292232;line-height:1;cursor:pointer}
.circle-icon{display:grid;place-items:center;width:42px;height:42px;border:1px solid #d9d1e0;border-radius:50%;background:#fff;transition:background .18s,color .18s,transform .18s}.circle-icon svg{width:18px;height:18px;fill:currentColor}
.caption{font-size:9px;line-height:1.3;letter-spacing:.025em}.social:hover .circle-icon,summary:hover .circle-icon,details[open] summary .circle-icon{background:#2d253b;color:#fff;border-color:#2d253b;transform:translateY(-2px)}
.like{display:inline-flex;align-items:center;gap:10px;min-height:48px;padding:6px 15px 6px 7px;background:#ede6f5;color:#65458e;border:1px solid #ded1eb;border-radius:30px;white-space:nowrap;font-size:12px;font-weight:700}.heart{display:grid;place-items:center;width:34px;height:34px;background:#fff;border-radius:50%}.heart svg{width:17px;height:17px;fill:none;stroke:currentColor;stroke-width:1.8}.count{min-width:13px;font-size:13px;font-variant-numeric:tabular-nums}.like[aria-pressed=true]{background:#65458e;border-color:#65458e;color:#fff}.like[aria-pressed=true] .heart{background:#ffffff20}.like[aria-pressed=true] svg{fill:currentColor}
.status{flex-basis:100%;font-size:12px;line-height:1.7;color:#685274;margin:0}.status:empty{display:none}:focus-visible{outline:3px solid #684497;outline-offset:4px}
details{position:relative}summary{list-style:none}summary::-webkit-details-marker{display:none}.options{position:absolute;right:0;top:calc(100% + 14px);z-index:50;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;width:min(320px,calc(100vw - 32px));padding:20px;background:#fff;border:1px solid #ded5e7;border-radius:14px;box-shadow:0 18px 50px #30204026}.options strong{grid-column:1/-1;font-size:10px;letter-spacing:.15em;color:#745891}.options .social{flex-direction:row;font-size:12px;min-height:44px}.options .caption{font-size:12px}
@container(max-width:600px){.row{flex-wrap:wrap;gap:20px}.with-like .socials{padding-left:0;border-left:0;width:100%}.circle{padding:20px 0}.socials{gap:7px}.social,summary{min-width:38px}.circle-icon{width:38px;height:38px}}
@media(prefers-reduced-motion:reduce){.circle-icon{transition:none;transform:none}}
:host([placement=floating]){position:fixed;left:0;right:0;bottom:0;z-index:10010;background:#fff;transform:translateY(110%);transition:transform .25s;container-type:normal}:host([placement=floating].on){transform:translateY(0)}:host([placement=floating]) .circle{justify-content:center;padding:10px 12px calc(10px + env(safe-area-inset-bottom))}@media(min-width:769px){:host([placement=floating]){display:none}}
`;

/** Circular sharing and reaction controls. The host owns reaction persistence. */
export class CircularShareBarView {
  static render(vm: ShareBarViewModel, attribute: string, caption: string, like: boolean): string {
    return render(vm, attribute, caption, like);
  }
}
