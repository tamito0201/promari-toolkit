/**
 * Render a view model as HTML without domain decisions.
 */
import type { ShareButtonViewModel, ShareBarViewModel } from '../application/BuildShareBarUseCase.ts';
import { HtmlEscaper } from './HtmlEscaper.ts';

type AttributeValue = string | boolean | null | undefined;

const attributes = (pairs: Readonly<Record<string, AttributeValue>>): string =>
  Object.entries(pairs)
    .filter(([, v]) => v !== null && v !== undefined && v !== false)
    .map(([k, v]) => (v === true ? ` ${k}` : ` ${k}="${HtmlEscaper.escape(String(v))}"`))
    .join('');

const content = (b: ShareButtonViewModel): string =>
  (b.labelStyle !== 'text' ? `<i>${b.icon}</i>` : '') + (b.labelStyle !== 'icon' ? `<span>${HtmlEscaper.escape(b.label)}</span>` : '');

const button = (trackAttribute: string) => (b: ShareButtonViewModel): string => {
  const common = {
    class: `${b.key} ${b.labelStyle}`,
    [trackAttribute]: b.key,
    'data-key': b.key,
    title: b.tooltip,
    'aria-label': b.tooltip,
    style: `--b:${b.color}`,
  };
  // compose copies and opens a tab in one click; as a link it would navigate without the copy.
  if (b.action === 'compose') return `<button${attributes({ type: 'button', ...common })}>${content(b)}</button>`;
  return `<a${attributes({
    ...common,
    href: b.href,
    target: b.newTab ? '_blank' : null,
    rel: b.action === 'open' ? ['noopener', 'noreferrer', b.nofollow ? 'nofollow' : null].filter(Boolean).join(' ') : null,
  })}>${content(b)}</a>`;
};

const render = (vm: ShareBarViewModel, trackAttribute: string, css: string): string => {
  const draw = button(trackAttribute);
  const group = (cls: string, items: readonly ShareButtonViewModel[]): string => (items.length ? `<span class="${cls}">${items.map(draw).join('')}</span>` : '');
  return `<style>${css}</style><div class="w${vm.headingPosition === 'top' ? ' top' : ''}" role="group" aria-label="${HtmlEscaper.escape(vm.groupLabel)}">`
    + (vm.heading ? `<span class="h">${HtmlEscaper.escape(vm.heading)}</span>` : '')
    + group('p', vm.primary) + group('s', vm.secondary) + '</div>';
};

/** The default share bar. */
export class ShareBarView {
  static render(vm: ShareBarViewModel, trackAttribute: string, css: string): string {
    return render(vm, trackAttribute, css);
  }
}
