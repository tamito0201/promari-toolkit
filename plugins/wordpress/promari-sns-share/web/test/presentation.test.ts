/**
 * Presentation styles and markup: the accent attribute is the only outside color input, and compose
 * destinations render as buttons.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { ShareBarStylesheet } from '../src/presentation/ShareBarStylesheet.ts';
import { BuildShareBarUseCase } from '../src/application/BuildShareBarUseCase.ts';
import { ShareButtonCatalog } from '../src/application/ShareButtonCatalog.ts';
import { CircularShareBarView } from '../src/presentation/CircularShareBarView.ts';
import { ShareBarView } from '../src/presentation/ShareBarView.ts';
import { ToastNotifier } from '../src/presentation/ToastNotifier.ts';
import { CONFIG, SPECS, CARD_SPEC, memoryRepository } from './domain.test.ts';

describe('ShareBarStylesheet', () => {
  it('--accent を :host ではなく内側で定義する（ページ側の --accent に上書きされない）', () => {
    const css = ShareBarStylesheet.build(CONFIG);
    const host = css.match(/:host\{[^}]*\}/)?.[0] ?? '';
    assert.doesNotMatch(host, /--accent/);
    assert.match(css, /\.w\{--accent:#54347e;/);
  });
});

describe('compose の描画', () => {
  const vm = new BuildShareBarUseCase(new ShareButtonCatalog(memoryRepository(SPECS), SPECS))
    .execute({ ...CONFIG, destinations: ['x', 'qiita'], secondary: ['copy'] }, { url: 'https://a.jp/', title: 'T', site: 'S', placement: 'inline', canNativeShare: false });

  it('既定の共有欄は compose をリンクではなくボタンで描き、href・target を持たせない', () => {
    const html = ShareBarView.render(vm, 'data-share', '');
    const qiita = html.match(/<(\w+)[^>]*data-key="qiita"[^>]*>/)!;
    assert.equal(qiita[1], 'button');
    assert.match(qiita[0], /type="button"/);
    assert.doesNotMatch(qiita[0], /href=|target=/);
    assert.match(html, /<a[^>]*data-key="x"[^>]*href="https:\/\/twitter\.com/);
  });
  it('円い共有欄も compose をボタンで描き、表に無いアイコンは TOML の icon を使う', () => {
    const html = CircularShareBarView.render(vm, 'data-share', '', false);
    const qiita = html.match(/<(\w+)[^>]*data-key="qiita"[^>]*>(.*?)<\/\1>/)!;
    assert.equal(qiita[1], 'button');
    assert.ok(qiita[2]!.includes(SPECS[4]!.icon));
  });
  it('既存の copy・native の描き方は変えない（既定の共有欄はリンク、円い共有欄はボタン）', () => {
    const both = new BuildShareBarUseCase(new ShareButtonCatalog(memoryRepository(SPECS), SPECS))
      .execute({ ...CONFIG, destinations: ['x', 'copy', 'native'], secondary: [] }, { url: 'https://a.jp/', title: 'T', site: 'S', placement: 'inline', canNativeShare: true });
    const tags = (html: string) => Object.fromEntries(['x', 'copy', 'native'].map((key) => [key, html.match(new RegExp(`<(\\w+)[^>]*data-key="${key}"`))![1]]));
    assert.deepEqual(tags(ShareBarView.render(both, 'data-share', '')), { x: 'a', copy: 'a', native: 'a' });
    assert.deepEqual(tags(CircularShareBarView.render(both, 'data-share', '', false)), { x: 'a', copy: 'button', native: 'button' });
    // rel は共有 URL をたどるリンク（open）だけに付く。
    assert.match(ShareBarView.render(both, 'data-share', ''), /data-key="x"[^>]*rel="noopener noreferrer nofollow"/);
    assert.doesNotMatch(ShareBarView.render(both, 'data-share', ''), /data-key="copy"[^>]*rel=/);
  });
  it('ボタンにもリンクと同じ見た目の規則を当てる', () => {
    const css = ShareBarStylesheet.build(CONFIG);
    assert.match(css, /\.p :is\(a,button\)\{height:/);
    assert.match(css, /\.s :is\(a,button\)\{width:/);
    assert.match(css, /:is\(a,button\)\.done\{/);
  });
});

describe('下書きを送る open の描画（ADR-0004）', () => {
  const specs = [...SPECS, CARD_SPEC];
  const vm = new BuildShareBarUseCase(new ShareButtonCatalog(memoryRepository(specs), specs))
    .execute({ ...CONFIG, destinations: ['cardblog'], secondary: [] }, { url: 'https://a.jp/', title: 'T', site: 'S', placement: 'inline', canNativeShare: false });

  it('既定の共有欄も円い共有欄も、投稿画面へのリンクとして描く', () => {
    for (const html of [ShareBarView.render(vm, 'data-share', ''), CircularShareBarView.render(vm, 'data-share', '', false)]) {
      const tag = html.match(/<(\w+)[^>]*data-key="cardblog"[^>]*>/)!;
      assert.equal(tag[1], 'a');
      assert.match(tag[0], /href="https:\/\/blog\.example\/new\?t=T&amp;body=%3Ca/);
      assert.match(tag[0], /rel="noopener noreferrer nofollow"/);
    }
  });
});

describe('ToastNotifier', () => {
  it('短い通知は1.8秒、長い通知（compose の案内つき）は長めに出し、6秒で打ち切る', () => {
    assert.equal(ToastNotifier.duration('コピーしました'), 1800);
    assert.ok(ToastNotifier.duration('タイトルとURLをコピーしました。投稿画面に貼り付けてください 貼り付けたあと Enter を押すと、記事のカードになります') > 3000);
    assert.equal(ToastNotifier.duration('あ'.repeat(500)), 6000);
  });
});
