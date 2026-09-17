/**
 * Render a view model as HTML without domain decisions.
 */
import type { ButtonViewModel, ShareBarViewModel } from '../application/BuildShareBar.ts';
import { Action } from '../domain/types.ts';

const ESCAPES: Readonly<Record<string, string>> = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' };
export const escapeHtml = (value: string): string => value.replace(/[&<>"]/g, (c) => ESCAPES[c] ?? c);

type AttributeValue = string | boolean | null | undefined;

const attributes = (pairs: Readonly<Record<string, AttributeValue>>): string =>
  Object.entries(pairs)
    .filter(([, v]) => v !== null && v !== undefined && v !== false)
    .map(([k, v]) => (v === true ? ` ${k}` : ` ${k}="${escapeHtml(String(v))}"`))
    .join('');

const button = (trackAttribute: string) => (b: ButtonViewModel): string =>
  `<a${attributes({
    class: `${b.key} ${b.labelStyle}`,
    href: b.href,
    [trackAttribute]: b.key,
    'data-key': b.key,
    title: b.tooltip,
    'aria-label': b.tooltip,
    style: `--b:${b.color}`,
    target: b.newTab ? '_blank' : null,
    rel: b.action === Action.Open ? ['noopener', 'noreferrer', b.nofollow ? 'nofollow' : null].filter(Boolean).join(' ') : null,
  })}>${b.labelStyle !== 'text' ? `<i>${b.icon}</i>` : ''}${b.labelStyle !== 'icon' ? `<span>${escapeHtml(b.label)}</span>` : ''}</a>`;

export const renderHtml = (vm: ShareBarViewModel, trackAttribute: string, css: string): string => {
  const render = button(trackAttribute);
  const group = (cls: string, items: readonly ButtonViewModel[]): string => (items.length ? `<span class="${cls}">${items.map(render).join('')}</span>` : '');
  return `<style>${css}</style><div class="w${vm.headingPosition === 'top' ? ' top' : ''}" role="group" aria-label="${escapeHtml(vm.groupLabel)}">`
    + (vm.heading ? `<span class="h">${escapeHtml(vm.heading)}</span>` : '')
    + group('p', vm.primary) + group('s', vm.secondary) + '</div>';
};
