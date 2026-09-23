/**
 * Application tests using plain-object gateways without a DOM.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { BuildShareBarUseCase } from '../src/application/BuildShareBarUseCase.ts';
import { DisplayCatalog } from '../src/application/DisplayCatalog.ts';
import { HandleShareClickUseCase } from '../src/application/HandleShareClickUseCase.ts';
import type { ShareActivity } from '../src/domain/gateway/ShareActivityPublisher.ts';
import type { ShareGateways } from '../src/domain/gateway/ShareGateways.ts';
import { AttributeConfigReader } from '../src/presentation/AttributeConfigReader.ts';
import { CONFIG, SPECS, memoryRepository } from './domain.test.ts';

const catalog = new DisplayCatalog(memoryRepository(SPECS), SPECS);
const buildShareBar = new BuildShareBarUseCase(catalog);
const input = { url: 'https://a.jp/post/', title: 'Hello', site: 'Promari', placement: 'inline' as const, canNativeShare: false };

describe('BuildShareBarUseCase', () => {
  it('表示用カタログは同じ共有先を公開し、表示メタデータを不変に保つ', () => {
    assert.deepEqual(catalog.keys(), SPECS.map(s => s.key));
    assert.equal(catalog.has('missing'), false);
    const service = catalog.resolve(['x'])[0]!;
    assert.deepEqual(service.appearance, { label: SPECS[0]!.label, color: SPECS[0]!.color, icon: SPECS[0]!.icon });
    assert.ok(Object.isFrozen(service.appearance));
    assert.throws(() => catalog.resolve(['missing']));
  });
  it('表示情報の無い共有先は黙って空欄にせず止める', () => {
    const partial = new DisplayCatalog(memoryRepository(SPECS), SPECS.filter((s) => s.key !== 'x'));
    assert.throws(() => partial.resolve(['x']), /表示情報がありません/);
  });
  it('主役と補助のビューモデルを作り、文言・色・UTM・popup を設定どおりに写す', () => {
    const vm = buildShareBar.execute(CONFIG, input);
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
    assert.equal(buildShareBar.execute(CONFIG, { ...input, placement: 'floating' }).heading, '');
  });
});

describe('HandleShareClickUseCase', () => {
  const gateways = (overrides: Partial<ShareGateways> = {}) => {
    const calls: string[] = [];
    const tracked: ShareActivity[] = [];
    const base: ShareGateways = {
      clipboard: { write: async (t) => { calls.push(`copy:${t}`); }, fallback: (t) => calls.push(`fallback:${t}`) },
      nativeShare: { available: true, share: async (d) => { calls.push(`share:${d.url}`); } },
      popup: { open: (href) => { calls.push(`popup:${href}`); return true; } },
      activity: { publish: (d) => tracked.push(d) },
    };
    return { calls, tracked, gateways: { ...base, ...overrides } };
  };
  const button = (key: string) => buildShareBar.execute({ ...CONFIG, secondary: ['copy', 'native'] }, { ...input, canNativeShare: true });

  it('copy はクリップボードへ書いて copied を返し、既定動作を止める', async () => {
    const { calls, tracked, gateways: g } = gateways();
    let prevented = false;
    const b = button('copy').secondary.find((x) => x.key === 'copy')!;
    const outcome = await new HandleShareClickUseCase(g).execute({ button: b, placement: 'inline', preventDefault: () => { prevented = true; } });
    assert.equal(outcome, 'copied');
    assert.deepEqual(calls, ['copy:https://a.jp/post/']);
    assert.equal(prevented, true);
    assert.deepEqual(tracked, [{ service: 'copy', url: 'https://a.jp/post/', placement: 'inline' }]);
  });
  it('書き込みの完了を待ってから結果を返す（完了前に成功を名乗らない）', async () => {
    let finish!: () => void;
    const outcomes: string[] = [];
    const { gateways: g } = gateways({ clipboard: { write: () => new Promise<void>(resolve => { finish = resolve; }) } });
    const b = button('copy').secondary.find(x => x.key === 'copy')!;
    const pending = new HandleShareClickUseCase(g).execute({ button: b, placement: 'inline', preventDefault: () => undefined }).then((o) => { outcomes.push(o); });
    await Promise.resolve();
    assert.deepEqual(outcomes, []);
    finish();
    await pending;
    assert.deepEqual(outcomes, ['copied']);
  });
  it('clipboard が失敗したら fallback を出し、copied とは返さない', async () => {
    const { calls, gateways: g } = gateways({ clipboard: { write: async () => { throw new Error('denied'); }, fallback: (t) => calls.push(`fallback:${t}`) } });
    const b = button('copy').secondary.find((x) => x.key === 'copy')!;
    const outcome = await new HandleShareClickUseCase(g).execute({ button: b, placement: 'inline', preventDefault: () => undefined });
    assert.equal(outcome, 'copy-fallback');
    assert.deepEqual(calls, ['fallback:https://a.jp/post/']);
  });
  it('端末共有の成功と拒否で、従来どおり操作イベントを一度だけ通知する', async () => {
    for (const reject of [false, true]) {
      const { gateways: g, tracked } = gateways({ nativeShare: { available: true, share: async () => { if (reject) throw new Error('cancel'); } } });
      let prevented = false;
      const b = button('native').secondary.find(x => x.key === 'native')!;
      const outcome = await new HandleShareClickUseCase(g).execute({ button: b, placement: 'inline', preventDefault: () => { prevented = true; } });
      assert.equal(outcome, reject ? 'share-dismissed' : 'shared');
      assert.equal(prevented, true);
      assert.deepEqual(tracked, [{ service: 'native', url: input.url, placement: 'inline' }]);
    }
  });
  it('popup が開けなければ既定動作（リンク遷移）に任せる', async () => {
    const { gateways: g } = gateways({ popup: { open: () => false } });
    let prevented = false;
    const b = button('x').primary.find((x) => x.key === 'x')!;
    const outcome = await new HandleShareClickUseCase(g).execute({ button: b, placement: 'inline', preventDefault: () => { prevented = true; } });
    assert.equal(outcome, 'follow');
    assert.equal(prevented, false);
  });
});

describe('AttributeConfigReader', () => {
  const fakeElement = (attrs: Record<string, string>): Element =>
    ({ hasAttribute: (n: string) => n in attrs, getAttribute: (n: string) => attrs[n] ?? null }) as unknown as Element;

  it('deepMerge は入れ子を部分的に上書きし、配列は置き換える', () => {
    const merged = AttributeConfigReader.merge(CONFIG, { appearance: { size: 'large' }, services: ['x'] });
    assert.equal(merged.appearance.size, 'large');
    assert.equal(merged.appearance.shape, 'official');
    assert.deepEqual(merged.services, ['x']);
  });
  it('個別属性 < config 属性 の優先順位で上書きする', () => {
    const config = new AttributeConfigReader(CONFIG).read(fakeElement({ services: 'x, facebook', size: 'large', utm: 'off', config: '{"appearance":{"size":"small"}}' }));
    assert.deepEqual(config.services, ['x', 'facebook']);
    assert.equal(config.appearance.size, 'small');
    assert.equal(config.utm.enabled, false);
  });
  it('壊れた JSON は無視して属性までの設定を返す', () => {
    const errors: unknown[] = [];
    const original = console.error;
    console.error = (...a: unknown[]) => { errors.push(a); };
    try {
      assert.equal(new AttributeConfigReader(CONFIG).read(fakeElement({ heading: 'X', config: '{oops' })).heading, 'X');
    } finally {
      console.error = original;
    }
    assert.equal(errors.length, 1);
  });
});
