/**
 * Pure domain tests, executable with node --test.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { createCatalog, UnknownServiceError } from '../src/domain/Service.ts';
import { createShareRequest } from '../src/domain/ShareRequest.ts';
import { appendQuery, decideClick, fillTemplate, selectServices, utmUrl } from '../src/domain/policies.ts';
import type { ServiceDefinition } from '../src/application/ServiceCatalog.ts';
import type { ShareConfig } from '../src/application/config.ts';
import { Action } from '../src/domain/types.ts';

const svg = '<svg><path fill="currentColor"/></svg>';
export const SPECS: readonly ServiceDefinition[] = [
  { key: 'x', label: 'ポスト', color: '#000000', icon: svg, action: Action.Open, endpoint: 'https://twitter.com/intent/tweet', params: { url: 'url', text: 'text', hashtags: 'hashtagsCsv', via: 'via' } },
  { key: 'facebook', label: 'シェア', color: '#1877F2', icon: svg, action: Action.Open, endpoint: 'https://www.facebook.com/sharer/sharer.php', params: { u: 'url' } },
  { key: 'copy', label: 'URLをコピー', color: '#5F6368', icon: svg, action: Action.Copy, endpoint: '', params: {} },
  { key: 'native', label: 'その他', color: '#5F6368', icon: svg, action: Action.Native, endpoint: '', params: {} },
];

export const CONFIG: ShareConfig = {
  services: ['facebook', 'x'],
  secondary: ['copy', 'native'],
  labels: { x: 'ポストする' },
  heading: 'SHARE',
  buttons: { copy: { floating: false } },
  appearance: { size: 'small', label_style: 'icon_text', shape: 'official', gap_px: 6, font_family: 'sans-serif', heading_position: 'left', secondary_size_px: 28, secondary_style: 'mono' },
  text: { title_template: '{title} | {site}', hashtags: ['promari'], via: '@promari_jp' },
  utm: { enabled: true, source: '{service}', medium: 'social', campaign: 'share', content: '' },
  behavior: { open_in_new_tab: true, popup: true, popup_width: 600, popup_height: 500, nofollow: true },
  floating: { position: 'bottom', after: 400, secondaryMax: 1, hideNearEnd: true, services: [] },
  tracking: { attribute: 'data-share', event_name: 'promari-sns-share' },
  messages: { copied: 'コピーしました', group_label: 'シェア' },
  style: { accent: '#54347e', floating_background: '#fff' },
};

describe('policies', () => {
  it('テンプレートを展開する', () => {
    assert.equal(fillTemplate('{title} | {site} {url}', { url: 'U', title: 'T', site: 'S' }), 'T | S U');
  });
  it('フラグメントを保って query を足す', () => {
    assert.equal(appendQuery('https://a.jp/p?x=1#h', { a: 'b c' }), 'https://a.jp/p?x=1&a=b%20c#h');
    assert.equal(appendQuery('https://a.jp/p', { a: '1' }), 'https://a.jp/p?a=1');
  });
  it('utm は enabled のときだけ・空の項目は付けない・{service} を置換する', () => {
    assert.equal(utmUrl('https://a.jp/', { ...CONFIG.utm, enabled: false }, 'x'), 'https://a.jp/');
    assert.equal(utmUrl('https://a.jp/', CONFIG.utm, 'x'), 'https://a.jp/?utm_source=x&utm_medium=social&utm_campaign=share');
  });
  it('native は端末が対応するときだけ・固定バーは floating=false を除き最大数で切る', () => {
    const catalog = createCatalog(SPECS);
    assert.deepEqual(selectServices(CONFIG, { placement: 'inline', canNativeShare: false, catalog }).secondary, ['copy']);
    assert.deepEqual(selectServices(CONFIG, { placement: 'inline', canNativeShare: true, catalog }).secondary, ['copy', 'native']);
    assert.deepEqual(selectServices(CONFIG, { placement: 'floating', canNativeShare: true, catalog }).secondary, ['native']);
    assert.deepEqual(selectServices({ ...CONFIG, floating: { ...CONFIG.floating, services: ['x'] } }, { placement: 'floating', canNativeShare: true, catalog }).primary, ['x']);
  });
  it('クリックの判断', () => {
    assert.equal(decideClick({ action: Action.Copy, popup: null }), 'copy');
    assert.equal(decideClick({ action: Action.Native, popup: null }), 'native');
    assert.equal(decideClick({ action: Action.Open, popup: { width: 1, height: 1 } }), 'popup');
    assert.equal(decideClick({ action: Action.Open, popup: null }), 'follow');
  });
});

describe('Service / Catalog', () => {
  it('ドメインの共有先へ表示用メタデータを持ち込まない', () => {
    const service = createCatalog(SPECS).resolve(['x'])[0]!;
    assert.deepEqual(Object.keys(service).sort(), ['action', 'key', 'shareUrl']);
    assert.ok(Object.isFrozen(service));
  });
  it('RFC 3986 でエンコードし、空の値は送らない', () => {
    const [x] = createCatalog(SPECS).resolve(['x']);
    const request = createShareRequest({ url: 'https://a.jp/?q=1', title: 'T', text: 'a b+c', hashtags: ['p', 'q'], via: '@v' });
    assert.equal(x!.shareUrl(request), 'https://twitter.com/intent/tweet?url=https%3A%2F%2Fa.jp%2F%3Fq%3D1&text=a%20b%2Bc&hashtags=p%2Cq&via=v');
    const empty = createShareRequest({ url: 'https://a.jp/', title: '', text: '' });
    assert.equal(x!.shareUrl(empty), 'https://twitter.com/intent/tweet?url=https%3A%2F%2Fa.jp%2F');
  });
  it('endpoint の無いサービスはページ URL を返す', () => {
    const [copy] = createCatalog(SPECS).resolve(['copy']);
    assert.equal(copy!.shareUrl(createShareRequest({ url: 'https://a.jp/', title: '', text: '' })), 'https://a.jp/');
  });
  it('未知の名前は例外（黙って読み飛ばさない）', () => {
    assert.throws(() => createCatalog(SPECS).resolve(['nope']), UnknownServiceError);
  });
});
