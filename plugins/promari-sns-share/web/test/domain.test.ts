/**
 * Pure domain tests, executable with node --test.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { ShareService } from '../src/domain/model/ShareService.ts';
import { ShareRequest } from '../src/domain/model/ShareRequest.ts';
import { Action, type ServiceSpec } from '../src/domain/model/ShareTypes.ts';
import { UnknownServiceError, type ShareServiceRepository } from '../src/domain/repository/ShareServiceRepository.ts';
import { ClickPolicy } from '../src/domain/service/ClickPolicy.ts';
import { ServiceSelectionPolicy } from '../src/domain/service/ServiceSelectionPolicy.ts';
import { ShareTextPolicy } from '../src/domain/service/ShareTextPolicy.ts';
import { UriEncoder } from '../src/domain/service/UriEncoder.ts';
import { UtmPolicy } from '../src/domain/service/UtmPolicy.ts';
import type { ServiceDefinition } from '../src/application/DisplayCatalog.ts';
import type { ShareConfig } from '../src/application/ShareConfig.ts';

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

/** ドメインのテストでは、リポジトリのインターフェースを満たす手書きの代役を使う。 */
export const memoryRepository = (specs: readonly ServiceSpec[]): ShareServiceRepository => {
  const services = new Map(specs.map((spec) => [spec.key, new ShareService(spec)] as const));
  return {
    has: (key) => services.has(key),
    keys: () => [...services.keys()],
    resolve: (keys) => keys.map((key) => services.get(key) ?? (() => { throw new UnknownServiceError(key); })()),
  };
};

describe('ドメインサービス', () => {
  it('テンプレートを展開する', () => {
    assert.equal(ShareTextPolicy.fill('{title} | {site} {url}', { url: 'U', title: 'T', site: 'S' }), 'T | S U');
    // 置換した結果は走査し直さない。題名の {site} は題名のまま残る。
    assert.equal(ShareTextPolicy.fill('{title} | {site}', { url: 'U', title: '特集{site}', site: 'S' }), '特集{site} | S');
  });
  it('フラグメントを保って query を足す', () => {
    assert.equal(UriEncoder.appendQuery('https://a.jp/p?x=1#h', { a: 'b c' }), 'https://a.jp/p?x=1&a=b%20c#h');
    assert.equal(UriEncoder.appendQuery('https://a.jp/p', { a: '1' }), 'https://a.jp/p?a=1');
    // encodeURIComponent が残す ! ' ( ) * も、RFC 3986 の予約文字なので符号化する。
    assert.equal(UriEncoder.appendQuery('https://a.jp/p', { 'k!': "v'(*)" }), 'https://a.jp/p?k%21=v%27%28%2A%29');
  });
  it('mailto は小窓で開かない', () => {
    assert.equal(ClickPolicy.canOpenInPopup('mailto:someone@a.jp?subject=x'), false);
    assert.equal(ClickPolicy.canOpenInPopup('https://a.jp/'), true);
  });
  it('utm は enabled のときだけ・空の項目は付けない・{service} を置換する', () => {
    assert.equal(UtmPolicy.apply('https://a.jp/', { ...CONFIG.utm, enabled: false }, 'x'), 'https://a.jp/');
    assert.equal(UtmPolicy.apply('https://a.jp/', CONFIG.utm, 'x'), 'https://a.jp/?utm_source=x&utm_medium=social&utm_campaign=share');
  });
  it('native は端末が対応するときだけ・固定バーは floating=false を除き最大数で切る', () => {
    const repository = memoryRepository(SPECS);
    assert.deepEqual(ServiceSelectionPolicy.select(CONFIG, { placement: 'inline', canNativeShare: false, repository }).secondary, ['copy']);
    assert.deepEqual(ServiceSelectionPolicy.select(CONFIG, { placement: 'inline', canNativeShare: true, repository }).secondary, ['copy', 'native']);
    assert.deepEqual(ServiceSelectionPolicy.select(CONFIG, { placement: 'floating', canNativeShare: true, repository }).secondary, ['native']);
    assert.deepEqual(ServiceSelectionPolicy.select({ ...CONFIG, floating: { ...CONFIG.floating, services: ['x'] } }, { placement: 'floating', canNativeShare: true, repository }).primary, ['x']);
  });
  it('クリックの判断', () => {
    assert.equal(ClickPolicy.decide({ action: Action.Copy, popup: null }), 'copy');
    assert.equal(ClickPolicy.decide({ action: Action.Native, popup: null }), 'native');
    assert.equal(ClickPolicy.decide({ action: Action.Open, popup: { width: 1, height: 1 } }), 'popup');
    assert.equal(ClickPolicy.decide({ action: Action.Open, popup: null }), 'follow');
  });
});

describe('ShareService（値オブジェクト）', () => {
  it('ドメインの共有先へ表示用メタデータを持ち込まない', () => {
    const service = new ShareService(SPECS[0]!);
    assert.deepEqual(Object.keys(service).sort(), ['action', 'key']);
    assert.equal('appearance' in service, false);
    assert.ok(Object.isFrozen(service));
  });
  it('RFC 3986 でエンコードし、空の値は送らない', () => {
    const x = new ShareService(SPECS[0]!);
    const request = ShareRequest.create({ url: 'https://a.jp/?q=1', title: 'T', text: 'a b+c', hashtags: ['p', 'q'], via: '@v' });
    assert.equal(x!.shareUrl(request), 'https://twitter.com/intent/tweet?url=https%3A%2F%2Fa.jp%2F%3Fq%3D1&text=a%20b%2Bc&hashtags=p%2Cq&via=v');
    const empty = ShareRequest.create({ url: 'https://a.jp/', title: '', text: '' });
    assert.equal(x!.shareUrl(empty), 'https://twitter.com/intent/tweet?url=https%3A%2F%2Fa.jp%2F');
    // encodeURIComponent は ! ' ( ) * を残すので、RFC 3986 に合わせて補って符号化する。
    const tricky = ShareRequest.create({ url: 'https://a.jp/', title: '', text: "a!b'c(d)e*f" });
    assert.equal(x!.shareUrl(tricky), 'https://twitter.com/intent/tweet?url=https%3A%2F%2Fa.jp%2F&text=a%21b%27c%28d%29e%2Af');
  });
  it('endpoint の無いサービスはページ URL を返す', () => {
    const copy = new ShareService(SPECS[2]!);
    assert.equal(copy!.shareUrl(ShareRequest.create({ url: 'https://a.jp/', title: '', text: '' })), 'https://a.jp/');
  });
});
