/**
 * Map configuration to CSS with the same styling rules as PHP Render/Styles.
 */
import type { SecondaryStyle, Shape, ShareConfig, Size } from '../application/config.ts';

interface Metrics { readonly h: number; readonly f: number; readonly i: number; readonly px: number }

const metrics = (size: Size): Metrics => (size === 'large' ? { h: 28, f: 13, i: 16, px: 10 } : { h: 20, f: 11, i: 12, px: 8 });

const radius = (shape: Shape, service: string): string =>
  ({ pill: '9999px', rounded: '6px', square: '0', official: service === 'x' ? '9999px' : '3px' })[shape];

const secondaryCss = (style: SecondaryStyle): string =>
  ({
    mono: '.s a{background:#f1f1f3;color:#5f6368}.s a:hover{background:var(--b);color:#fff}',
    brand: '.s a{background:var(--b);color:#fff}.s a:hover{filter:brightness(1.1)}',
    outline: '.s a{background:transparent;color:var(--b);box-shadow:inset 0 0 0 1px var(--b)}.s a:hover{background:var(--b);color:#fff}',
  })[style];

export const buildCss = ({ appearance, style, floating }: ShareConfig): string => {
  const m = metrics(appearance.size);
  const s = appearance.secondary_size_px;
  const side = floating.position === 'top' ? 'top:0;transform:translateY(-110%)' : 'bottom:0;transform:translateY(110%)';
  return [
    `:host{display:block;font:${m.f}px/1 ${appearance.font_family};--accent:${style.accent}}`,
    '.w{display:flex;flex-wrap:wrap;align-items:center;gap:10px 14px}',
    '.w.top{flex-direction:column;align-items:flex-start;gap:8px}',
    '.h{font-size:11px;font-weight:700;letter-spacing:.12em;color:var(--accent);margin-right:2px}',
    `.p,.s{display:inline-flex;align-items:center;gap:${appearance.gap_px}px}`,
    '.s{padding-left:10px;border-left:1px solid rgba(0,0,0,.12)}',
    'a{display:inline-flex;align-items:center;gap:4px;box-sizing:border-box;text-decoration:none;color:#fff;white-space:nowrap;cursor:pointer;transition:filter .15s,transform .15s,background .15s}',
    'a:hover{filter:brightness(1.08)}a:active{transform:translateY(1px)}a:focus-visible{outline:2px solid var(--accent);outline-offset:2px}',
    `i{display:inline-flex;width:${m.i}px;height:${m.i}px}i svg{width:100%;height:100%;display:block}`,
    `.p a{height:${m.h}px;padding:0 ${m.px}px 0 ${m.px - 2}px;font-weight:700;font-size:${m.f}px;background:var(--b);border-radius:${radius(appearance.shape, '')}}`,
    `.p a.x{border-radius:${radius(appearance.shape, 'x')};padding:0 ${m.px + 2}px 0 ${m.px}px}`,
    `.p a.line i{width:${m.i + 2}px;height:${m.i + 2}px}.p a.icon{padding:0 ${m.px - 2}px}`,
    `.s a{width:${s}px;height:${s}px;justify-content:center;border-radius:50%;padding:0}.s a i{width:${Math.round(s * 0.54)}px;height:${Math.round(s * 0.54)}px}`,
    secondaryCss(appearance.secondary_style),
    'a.done{background:var(--accent)!important;color:#fff}',
    `:host([placement="floating"]){position:fixed;left:0;right:0;${side};z-index:10010;padding:10px 12px calc(10px + env(safe-area-inset-bottom));background:${style.floating_background};box-shadow:0 -6px 20px rgba(0,0,0,.08);transition:transform .25s ease}`,
    ':host([placement="floating"]) .w{justify-content:center;gap:8px}:host([placement="floating"].on){transform:translateY(0)}',
    '@media(min-width:769px){:host([placement="floating"]){display:none}}',
    '.t{position:fixed;left:50%;bottom:72px;z-index:10011;transform:translateX(-50%);padding:8px 14px;border-radius:999px;background:#1b1b1b;color:#fff;font-size:12px;opacity:0;transition:opacity .2s;pointer-events:none}.t.on{opacity:1}',
  ].join('');
};
