/**
 * Application tests using plain-object gateways without a DOM.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { BuildShareBarUseCase } from '../src/application/BuildShareBarUseCase.ts';
import { ShareButtonCatalog } from '../src/application/ShareButtonCatalog.ts';
import { HandleShareClickUseCase } from '../src/application/HandleShareClickUseCase.ts';
import type { ShareActivity } from '../src/domain/gateway/ShareActivityPublisher.ts';
import type { ShareGateways } from '../src/domain/gateway/ShareGateways.ts';
import { ShareSettingsAttributeReader } from '../src/presentation/ShareSettingsAttributeReader.ts';
import { CONFIG, SPECS, memoryRepository } from './domain.test.ts';
import { CATALOG } from '../src/generated/catalog.ts';
import { LinkCardPolicy } from '../src/domain/service/LinkCardPolicy.ts';

const catalog = new ShareButtonCatalog(memoryRepository(SPECS), SPECS);
const buildShareBar = new BuildShareBarUseCase(catalog);
const input = { url: 'https://a.jp/post/', title: 'Hello', site: 'Promari', placement: 'inline' as const, canNativeShare: false };

describe('BuildShareBarUseCase', () => {
  it('表示用カタログは同じ共有先を公開し、表示メタデータを不変に保つ', () => {
    assert.deepEqual(catalog.keys(), SPECS.map(s => s.key));
    assert.equal(catalog.has('missing'), false);
    const destination = catalog.resolve(['x'])[0]!;
    assert.deepEqual(destination.appearance, { label: SPECS[0]!.label, color: SPECS[0]!.color, icon: SPECS[0]!.icon, composeHint: '' });
    assert.ok(Object.isFrozen(destination.appearance));
    assert.throws(() => catalog.resolve(['missing']));
  });
  it('表示情報の無い共有先は黙って空欄にせず止める', () => {
    const partial = new ShareButtonCatalog(memoryRepository(SPECS), SPECS.filter((s) => s.key !== 'x'));
    assert.throws(() => partial.resolve(['x']), /表示情報がありません/);
  });
  it('未知の共有先キーは描画を止めず、外したうえで報告する', () => {
    const vm = buildShareBar.execute({ ...CONFIG, destinations: ['facebok', 'x'], secondary: ['copy', 'nope', 'facebok'] }, input);
    assert.deepEqual(vm.primary.map((b) => b.key), ['x']);
    assert.deepEqual(vm.secondary.map((b) => b.key), ['copy']);
    assert.deepEqual(vm.unknownDestinations, ['facebok', 'nope']);
    assert.deepEqual(buildShareBar.execute(CONFIG, input).unknownDestinations, []);
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
      newTab: { open: (href) => { calls.push(`tab:${href}`); } },
      activity: { publish: (d) => tracked.push(d) },
    };
    return { calls, tracked, gateways: { ...base, ...overrides } };
  };
  const button = (key: string) => buildShareBar.execute({ ...CONFIG, secondary: ['copy', 'native', 'qiita'] }, { ...input, canNativeShare: true });
  const compose = () => button('qiita').secondary.find((x) => x.key === 'qiita')!;

  it('copy はクリップボードへ書いて copied を返し、既定動作を止める', async () => {
    const { calls, tracked, gateways: g } = gateways();
    let prevented = false;
    const b = button('copy').secondary.find((x) => x.key === 'copy')!;
    const outcome = await new HandleShareClickUseCase(g).execute({ button: b, placement: 'inline', preventDefault: () => { prevented = true; } });
    assert.equal(outcome, 'copied');
    assert.deepEqual(calls, ['copy:https://a.jp/post/']);
    assert.equal(prevented, true);
    assert.deepEqual(tracked, [{ destination: 'copy', url: 'https://a.jp/post/', placement: 'inline' }]);
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
      assert.deepEqual(tracked, [{ destination: 'native', url: input.url, placement: 'inline' }]);
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

describe('HandleShareClickUseCase（compose）', () => {
  const recording = (write: (text: string) => Promise<void>) => {
    const calls: string[] = [];
    const tracked: ShareActivity[] = [];
    const g: ShareGateways = {
      clipboard: { write: (t) => { calls.push(`copy:${t}`); return write(t); }, fallback: (t) => calls.push(`fallback:${t}`) },
      nativeShare: { available: false, share: async () => undefined },
      popup: { open: (href) => { calls.push(`popup:${href}`); return true; } },
      newTab: { open: (href) => { calls.push(`tab:${href}`); } },
      activity: { publish: (d) => tracked.push(d) },
    };
    return { calls, tracked, g };
  };
  const qiita = () => buildShareBar.execute({ ...CONFIG, secondary: ['qiita'] }, input).secondary[0]!;

  it('ビューモデル: 投稿画面を href に、UTM 付きの「題名＋改行＋URL」を draft に持ち、小窓にしない', () => {
    const b = qiita();
    assert.equal(b.action, 'compose');
    assert.equal(b.href, 'https://qiita.com/drafts/new');
    assert.equal(b.draft, 'Hello\nhttps://a.jp/post/?utm_source=qiita&utm_medium=social&utm_campaign=share');
    assert.equal(b.popup, null);
    assert.equal(buildShareBar.execute({ ...CONFIG, secondary: ['copy'] }, input).secondary[0]!.draft, '');
  });
  it('クリップボードへの書き込みと新しいタブを、最初の await より前に同期で始める', () => {
    const { calls, g } = recording(() => new Promise<void>(() => undefined)); // never settles
    let prevented = false;
    void new HandleShareClickUseCase(g).execute({ button: qiita(), placement: 'inline', preventDefault: () => { prevented = true; } });
    // execute() の呼び出しから戻った時点（マイクロタスクを一度も回していない）で、両方が済んでいる。
    assert.deepEqual(calls, ['copy:Hello\nhttps://a.jp/post/?utm_source=qiita&utm_medium=social&utm_campaign=share', 'tab:https://qiita.com/drafts/new']);
    assert.equal(prevented, true);
  });
  it('書き込みが成功したら composed を返し、操作を一度だけ通知する', async () => {
    const { tracked, g } = recording(async () => undefined);
    const outcome = await new HandleShareClickUseCase(g).execute({ button: qiita(), placement: 'inline', preventDefault: () => undefined });
    assert.equal(outcome, 'composed');
    assert.deepEqual(tracked, [{ destination: 'qiita', url: input.url, placement: 'inline' }]);
  });
  it('書き込みが拒否されても・同期で投げても投稿画面は開き、compose-copy-failed を返す（prompt は出さない）', async () => {
    for (const write of [async () => { throw new Error('denied'); }, () => { throw new Error('sync'); }]) {
      const { calls, g } = recording(write as (t: string) => Promise<void>);
      const outcome = await new HandleShareClickUseCase(g).execute({ button: qiita(), placement: 'inline', preventDefault: () => undefined });
      assert.equal(outcome, 'compose-copy-failed');
      assert.ok(calls.includes('tab:https://qiita.com/drafts/new'));
      assert.equal(calls.some((c) => c.startsWith('fallback:') || c.startsWith('popup:')), false);
    }
  });
});

describe('同梱の共有先のリンクカード（destinations/*.toml, ADR-0004）', () => {
  const bundled = new BuildShareBarUseCase(new ShareButtonCatalog(memoryRepository(CATALOG), CATALOG));
  const page = { url: 'https://promari.jp/blog/share/', title: '共有ボタンを作る', site: 'Promari', description: '記事の説明 & 要点', image: 'https://promari.jp/cover.webp', placement: 'inline' as const, canNativeShare: false };
  const find = (key: string, overrides: Partial<typeof page> = {}) =>
    bundled.execute({ ...CONFIG, destinations: ['x'], secondary: ['ameba', 'qiita', 'zenn', 'medium', 'note'] }, { ...page, ...overrides }).secondary.find((b) => b.key === key)!;
  const query = (href: string) => new URLSearchParams(href.slice(href.indexOf('?') + 1));

  it('アメブロは open のリンクで、投稿画面に entry_title と entry_text（カードの HTML）を送る', () => {
    const ameba = find('ameba');
    assert.equal(ameba.action, 'open');
    assert.equal(ameba.linkable, true);
    assert.equal(ameba.followsLink, true);
    assert.ok(ameba.href.startsWith('https://blog.ameba.jp/ucs/entry/srventryinsertinput.do?entry_title='));
    const q = query(ameba.href);
    assert.equal(q.get('entry_title'), '共有ボタンを作る');
    const card = q.get('entry_text')!;
    assert.match(card, /^<div class="ogpCard_root">/);
    assert.ok(card.includes('href="https://promari.jp/blog/share/?utm_source=ameba&amp;utm_medium=social&amp;utm_campaign=share"'));
    assert.ok(card.includes('>共有ボタンを作る</span>'));
    assert.ok(card.includes('>記事の説明 &amp; 要点</span>'));
    assert.ok(card.includes('>promari.jp</span>'));
    assert.ok(card.includes('src="https://promari.jp/cover.webp"'));
    assert.ok(ameba.href.length <= LinkCardPolicy.URL_MAX, String(ameba.href.length));
    assert.equal(ameba.draft, '', 'open はコピーしない');
  });
  it('アメブロは小窓の設定があっても小窓にしない（投稿画面は小窓に狭い）', () => {
    assert.equal(CONFIG.behavior.popup, true);
    assert.equal(find('ameba').popup, null);
    assert.deepEqual(bundled.execute({ ...CONFIG, destinations: ['x'], secondary: [] }, page).primary[0]!.popup, { width: 600, height: 500 }, 'ほかの open は従来どおり');
  });
  it('上限は 3,500 文字。promari.jp の約70文字の題名（「| プロマリのブログ」付き）では画像を外さない', () => {
    assert.equal(LinkCardPolicy.URL_MAX, 3500);
    const title = `${'あ'.repeat(59)} | プロマリのブログ`;
    assert.equal(Array.from(title).length, 70);
    const ameba = find('ameba', { url: 'https://promari.jp/blog/share-buttons-plugin/', title, description: 'い'.repeat(60), image: 'https://promari.jp/wp-content/uploads/2026/09/share-buttons-plugin-cover.webp' });
    const card = query(ameba.href).get('entry_text')!;
    assert.ok(card.includes('src="https://promari.jp/wp-content/uploads/2026/09/share-buttons-plugin-cover.webp"'), '画像は残る');
    assert.equal(query(ameba.href).get('entry_title'), title, '題名は縮めない');
    assert.ok(ameba.href.length <= 3500, String(ameba.href.length));
  });
  it('アメブロの URL は、日本語の長い題名と説明でも 3,500 文字以内に収まる', () => {
    for (const title of ['あ'.repeat(40), 'あ'.repeat(100), 'あ'.repeat(400)]) {
      const href = find('ameba', { title, description: 'い'.repeat(200) }).href;
      assert.ok(href.length <= LinkCardPolicy.URL_MAX, `${title.length}: ${href.length}`);
      assert.match(query(href).get('entry_title')!, /^あ+…?$/);
    }
  });
  it('Qiita・Zenn・Medium のコピー文は、各サービスでカードになる書き方', () => {
    const utm = (key: string) => `https://promari.jp/blog/share/?utm_source=${key}&utm_medium=social&utm_campaign=share`;
    assert.equal(find('qiita').draft, `共有ボタンを作る\n\n${utm('qiita')}\n`);
    assert.equal(find('zenn').draft, `共有ボタンを作る\n\n@[card](${utm('zenn')})\n`);
    assert.equal(find('medium').draft, utm('medium'));
    assert.equal(find('medium').composeHint, '貼り付けたあと Enter を押すと、記事のカードになります');
    assert.equal(find('qiita').composeHint, '');
    assert.equal(find('note').href, 'https://note.com/intent/post?url=https%3A%2F%2Fpromari.jp%2Fblog%2Fshare%2F%3Futm_source%3Dnote%26utm_medium%3Dsocial%26utm_campaign%3Dshare&hashtags=promari');
  });
});

describe('ShareSettingsAttributeReader', () => {
  const fakeElement = (attrs: Record<string, string>): Element =>
    ({ hasAttribute: (n: string) => n in attrs, getAttribute: (n: string) => attrs[n] ?? null }) as unknown as Element;

  it('deepMerge は入れ子を部分的に上書きし、配列は置き換える', () => {
    const merged = ShareSettingsAttributeReader.merge(CONFIG, { appearance: { size: 'large' }, destinations: ['x'] });
    assert.equal(merged.appearance.size, 'large');
    assert.equal(merged.appearance.shape, 'official');
    assert.deepEqual(merged.destinations, ['x']);
  });
  it('deepMerge は base に無いキー（既定に無い共有先の labels・buttons）も残す', () => {
    const merged = ShareSettingsAttributeReader.merge(
      { ...CONFIG, labels: {}, buttons: {} },
      { labels: { hatena: 'はてブ' }, buttons: { hatena: { color: '#00a4de' } } },
    );
    assert.deepEqual(merged.labels, { hatena: 'はてブ' });
    assert.deepEqual(merged.buttons, { hatena: { color: '#00a4de' } });
  });
  it('after・列挙の属性は不正な値なら設定値を保つ', () => {
    const read = (attrs: Record<string, string>) => new ShareSettingsAttributeReader(CONFIG).read(fakeElement(attrs));
    for (const after of ['', 'abc', '-1', 'Infinity']) {
      assert.equal(read({ after }).floating.after, CONFIG.floating.after, `after=${JSON.stringify(after)}`);
    }
    assert.equal(read({ after: '0' }).floating.after, 0);
    assert.equal(read({ after: '250' }).floating.after, 250);
    assert.deepEqual(read({ size: 'huge', shape: 'circle', 'label-style': 'x', 'heading-position': 'bottom' }).appearance, CONFIG.appearance);
    assert.equal(read({ size: 'large' }).appearance.size, 'large');
  });
  it('個別属性 < config 属性 の優先順位で上書きする', () => {
    const config = new ShareSettingsAttributeReader(CONFIG).read(fakeElement({ destinations: 'x, facebook', size: 'large', utm: 'off', config: '{"appearance":{"size":"small"}}' }));
    assert.deepEqual(config.destinations, ['x', 'facebook']);
    assert.equal(config.appearance.size, 'small');
    assert.equal(config.utm.enabled, false);
  });
  it('壊れた JSON は無視して属性までの設定を返す', () => {
    const errors: unknown[] = [];
    const original = console.error;
    console.error = (...a: unknown[]) => { errors.push(a); };
    try {
      assert.equal(new ShareSettingsAttributeReader(CONFIG).read(fakeElement({ heading: 'X', config: '{oops' })).heading, 'X');
    } finally {
      console.error = original;
    }
    assert.equal(errors.length, 1);
  });
});
