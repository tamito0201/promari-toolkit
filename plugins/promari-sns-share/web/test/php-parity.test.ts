/**
 * Contract test: PHP and TypeScript must return the same result for the same input.
 * 片方だけ直すとここで落ちる。人の注意力ではなく機械で一致を保つ。
 */
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { describe, it } from 'node:test';
import { createCatalog } from '../src/domain/Service.ts';
import { createShareRequest } from '../src/domain/ShareRequest.ts';
import { appendQuery, canOpenInPopup, fillTemplate } from '../src/domain/policies.ts';
import { CATALOG } from '../src/generated/catalog.ts';

// encodeURIComponent と rawurlencode の差は ! ' ( ) * にだけ出る。
// 日本語や空白しか試さないと、食い違ったままテストが緑になる。
const TRICKY = "a!b'c(d)e*f~g+h/i?j=k&l m";
const KEYS: readonly string[] = CATALOG.map((service) => service.key);

const CASES = {
  shareUrl: KEYS.flatMap((service) => [
    { service, url: 'https://example.jp/post/?q=1#top', title: TRICKY, text: `${TRICKY} を読む`, hashtags: ['tag!', '日本語', ''], via: '@promari_jp', site: 'Promari' },
    { service, url: 'https://example.jp/', title: '', text: '', hashtags: [], via: '', site: '' },
  ]),
  template: [
    // 題名にプレースホルダが入っていても、置換した結果は走査し直さない。
    { template: '{title} | {site}', url: 'https://example.jp/', title: '特集{site}を読む', site: 'Promari' },
    { template: '{title} | {site} {url}', url: 'https://example.jp/', title: TRICKY, site: 'Promari' },
    { template: '{url}{url}', url: '{title}', title: 'T', site: 'S' },
    { template: 'プレースホルダなし', url: 'U', title: 'T', site: 'S' },
  ],
  appendQuery: [
    { url: 'https://example.jp/p?x=1#h', params: { utm_source: TRICKY } },
    { url: 'https://example.jp/p', params: { a: "b'c", d: 'e(f)' } },
    { url: 'https://example.jp/p#h', params: { 'k!': 'v*' } },
  ],
  popup: ['mailto:someone@example.jp?subject=a', 'https://example.jp/', 'https://twitter.com/intent/tweet'],
  request: [
    { url: 'https://example.jp/', title: 'T', text: 'X', hashtags: ['a', '', 'b!'], via: '@promari_jp', site: 'S' },
    { url: 'https://example.jp/', title: 'T', text: 'X', hashtags: [], via: 'promari_jp', site: 'S' },
  ],
};

const run = (): {
  serviceKeys: readonly string[];
  shareUrl: readonly (string | null)[];
  template: readonly string[];
  appendQuery: readonly string[];
  popup: readonly boolean[];
  request: readonly { via: string; hashtags: readonly string[]; hashtagsCsv: string }[];
} => {
  const script = fileURLToPath(new URL('../../tools/tests/parity_dump.php', import.meta.url));
  const php = spawnSync('php', [script], { input: JSON.stringify(CASES), encoding: 'utf8' });
  assert.equal(php.status, 0, `php parity_dump.php が失敗しました: ${php.stderr}`);
  return JSON.parse(php.stdout) as ReturnType<typeof run>;
};

describe('PHP と TypeScript の一致', () => {
  const expected = run();

  it('共有先の一覧が一致する', () => {
    assert.deepEqual([...KEYS].sort(), [...expected.serviceKeys].sort());
  });

  it('共有URLが一致する（! \' ( ) * を含む入力でも）', () => {
    const catalog = createCatalog(CATALOG);
    CASES.shareUrl.forEach((testCase, index) => {
      const [service] = catalog.resolve([testCase.service]);
      const request = createShareRequest(testCase);
      assert.equal(service!.shareUrl(request), expected.shareUrl[index], `${testCase.service} / ${testCase.title}`);
    });
  });

  it('テンプレートの展開が一致する（置換結果を走査し直さない）', () => {
    CASES.template.forEach((testCase, index) => {
      assert.equal(fillTemplate(testCase.template, testCase), expected.template[index], testCase.template);
    });
  });

  it('クエリの追加が一致する', () => {
    CASES.appendQuery.forEach((testCase, index) => {
      assert.equal(appendQuery(testCase.url, testCase.params), expected.appendQuery[index], testCase.url);
    });
  });

  it('小窓で開けるかの判断が一致する', () => {
    CASES.popup.forEach((href, index) => {
      assert.equal(canOpenInPopup(href), expected.popup[index], href);
    });
  });

  it('共有内容の正規化が一致する（@ の除去・空のハッシュタグ）', () => {
    CASES.request.forEach((testCase, index) => {
      const request = createShareRequest(testCase);
      assert.deepEqual({ via: request.via, hashtags: [...request.hashtags], hashtagsCsv: request.hashtagsCsv }, expected.request[index]);
    });
  });
});
