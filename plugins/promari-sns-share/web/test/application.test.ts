/**
 * Application tests using plain-object ports without a DOM.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { buildShareBar } from '../src/application/BuildShareBar.ts';
import { handleShareClick } from '../src/application/HandleShareClick.ts';
import type { Ports, TrackDetail } from '../src/application/ports.ts';
import { createDisplayCatalog } from '../src/application/ServiceCatalog.ts';
import { deepMerge, readConfig } from '../src/infrastructure/AttributeConfig.ts';
import { CONFIG, SPECS } from './domain.test.ts';

const catalog = createDisplayCatalog(SPECS);
const input = { url: 'https://a.jp/post/', title: 'Hello', site: 'Promari', placement: 'inline' as const, canNativeShare: false };

describe('buildShareBar', () => {
  it('表示用カタログは同じ共有先を公開し、表示メタデータを不変に保つ', () => {
    assert.deepEqual(catalog.keys(), SPECS.map(s => s.key));
    assert.equal(catalog.has('missing'), false);
    const service = catalog.resolve(['x'])[0]!;
    assert.deepEqual(service.appearance, { label: SPECS[0]!.label, color: SPECS[0]!.color, icon: SPECS[0]!.icon });
    assert.ok(Object.isFrozen(service.appearance));
    assert.throws(() => catalog.resolve(['missing']));
  });
  it('主役と補助のビューモデルを作り、文言・色・UTM・popup を設定どおりに写す', () => {
    const vm = buildShareBar({ catalog }, CONFIG, input);
    assert.equal(vm.heading, 'SHARE');
    assert.deepEqual(vm.primary.map((b) => b.key), ['facebook', 'x']);
    assert.deepEqual(vm.secondary.map((b) => b.key), ['copy']);
    const x = vm.primary[1]!;
    assert.equal(x.label, 'ポストする');
    assert.equal(x.color, '#000000');
    assert.match(x.href, /utm_source%3Dx/); // UTM parameters are inside the encoded page URL.
    assert.match(x.href, /text=Hello%20%7C%20Promari/);
    assert.match(x.href, /hashtags=promari&via=promari_jp/);
    assert.deepEqual(x.popup, { width: 600, height: 500 });
    assert.equal(x.newTab, true);
    assert.equal(vm.secondary[0]!.labelStyle, 'icon');
    assert.equal(vm.secondary[0]!.popup, null);
  });
  it('固定バーでは見出しを出さない', () => {
    assert.equal(buildShareBar({ catalog }, CONFIG, { ...input, placement: 'floating' }).heading, '');
  });
});

describe('handleShareClick', () => {
  const ports = (overrides: Partial<Ports> = {}) => {
    const calls: string[] = [];
    const tracked: TrackDetail[] = [];
    const base: Ports = {
      clipboard: { write: async (t) => { calls.push(`copy:${t}`); }, fallback: (t) => calls.push(`fallback:${t}`) },
      sharer: { available: true, share: async (d) => { calls.push(`share:${d.url}`); } },
      popup: { open: (href) => { calls.push(`popup:${href}`); return true; } },
      notifier: { notify: (t) => calls.push(`toast:${t}`) },
      tracker: { track: (d) => tracked.push(d) },
    };
    return { calls, tracked, ports: { ...base, ...overrides } };
  };
  const button = (key: string) => buildShareBar({ catalog }, { ...CONFIG, secondary: ['copy', 'native'] }, { ...input, canNativeShare: true });

  it('copy はクリップボードへ書いてトーストを出し、既定動作を止める', async () => {
    const { calls, tracked, ports: p } = ports();
    let prevented = false;
    const b = button('copy').secondary.find((x) => x.key === 'copy')!;
    await handleShareClick(p, CONFIG.messages)({ button: b, placement: 'inline', preventDefault: () => { prevented = true; } });
    assert.deepEqual(calls, ['copy:https://a.jp/post/', 'toast:コピーしました']);
    assert.equal(prevented, true);
    assert.deepEqual(tracked, [{ service: 'copy', url: 'https://a.jp/post/', placement: 'inline' }]);
  });
  it('非同期の成功を待ち、DOMを使わず通知先の識別子を渡す', async () => {
    let finish!: () => void;
    const notices: [string, string | undefined][] = [];
    const { ports: p } = ports({
      clipboard: { write: () => new Promise<void>(resolve => { finish = resolve; }) },
      notifier: { notify: (text, target) => { notices.push([text, target]); } },
    });
    const b = button('copy').secondary.find(x => x.key === 'copy')!;
    const pending = handleShareClick(p, CONFIG.messages)({ button: b, placement: 'inline', notificationTarget: 'target-2', preventDefault: () => undefined });
    assert.deepEqual(notices, []);
    finish();
    await pending;
    assert.deepEqual(notices, [['コピーしました', 'target-2']]);
  });
  it('clipboard が失敗したら fallback', async () => {
    const { calls, ports: p } = ports({ clipboard: { write: async () => { throw new Error('denied'); }, fallback: (t) => calls.push(`fallback:${t}`) } });
    const b = button('copy').secondary.find((x) => x.key === 'copy')!;
    await handleShareClick(p, CONFIG.messages)({ button: b, placement: 'inline', preventDefault: () => undefined });
    assert.deepEqual(calls, ['fallback:https://a.jp/post/']);
  });
  it('端末共有の成功と拒否で、従来どおり操作イベントを一度だけ通知する', async () => {
    for (const reject of [false, true]) {
      const { ports: p, tracked } = ports({ sharer: { available: true, share: async () => { if (reject) throw new Error('cancel'); } } });
      let prevented = false;
      const b = button('native').secondary.find(x => x.key === 'native')!;
      await handleShareClick(p, CONFIG.messages)({ button: b, placement: 'inline', preventDefault: () => { prevented = true; } });
      assert.equal(prevented, true);
      assert.deepEqual(tracked, [{ service: 'native', url: input.url, placement: 'inline' }]);
    }
  });
  it('popup が開けなければ既定動作（リンク遷移）に任せる', async () => {
    const { ports: p } = ports({ popup: { open: () => false } });
    let prevented = false;
    const b = button('x').primary.find((x) => x.key === 'x')!;
    await handleShareClick(p, CONFIG.messages)({ button: b, placement: 'inline', preventDefault: () => { prevented = true; } });
    assert.equal(prevented, false);
  });
});

describe('AttributeConfig', () => {
  const fakeElement = (attrs: Record<string, string>): Element =>
    ({ hasAttribute: (n: string) => n in attrs, getAttribute: (n: string) => attrs[n] ?? null }) as unknown as Element;

  it('deepMerge は入れ子を部分的に上書きし、配列は置き換える', () => {
    const merged = deepMerge(CONFIG, { appearance: { size: 'large' }, services: ['x'] });
    assert.equal(merged.appearance.size, 'large');
    assert.equal(merged.appearance.shape, 'official');
    assert.deepEqual(merged.services, ['x']);
  });
  it('個別属性 < config 属性 の優先順位で上書きする', () => {
    const config = readConfig(fakeElement({ services: 'x, facebook', size: 'large', utm: 'off', config: '{"appearance":{"size":"small"}}' }), CONFIG);
    assert.deepEqual(config.services, ['x', 'facebook']);
    assert.equal(config.appearance.size, 'small');
    assert.equal(config.utm.enabled, false);
  });
  it('壊れた JSON は無視して属性までの設定を返す', () => {
    const errors: unknown[] = [];
    const original = console.error;
    console.error = (...a: unknown[]) => { errors.push(a); };
    try {
      assert.equal(readConfig(fakeElement({ heading: 'X', config: '{oops' }), CONFIG).heading, 'X');
    } finally {
      console.error = original;
    }
    assert.equal(errors.length, 1);
  });
});
