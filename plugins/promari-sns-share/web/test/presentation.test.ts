/**
 * Presentation styles: the only way to change the accent color from outside is the accent attribute.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { ShareBarStyles } from '../src/presentation/ShareBarStyles.ts';
import { CONFIG } from './domain.test.ts';

describe('ShareBarStyles', () => {
  it('--accent を :host ではなく内側で定義する（ページ側の --accent に上書きされない）', () => {
    const css = ShareBarStyles.build(CONFIG);
    const host = css.match(/:host\{[^}]*\}/)?.[0] ?? '';
    assert.doesNotMatch(host, /--accent/);
    assert.match(css, /\.w\{--accent:#54347e;/);
  });
});
