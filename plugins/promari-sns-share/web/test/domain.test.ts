/**
 * Pure domain tests, executable with node --test.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { ShareDestination } from '../src/domain/model/ShareDestination.ts';
import { ShareRequest } from '../src/domain/model/ShareRequest.ts';
import { ShareAction } from '../src/domain/model/ShareAction.ts';
import { type ShareDestinationSpec } from '../src/domain/model/ShareDestinationSpec.ts';
import { UnknownShareDestinationError, type ShareDestinationRepository } from '../src/domain/repository/ShareDestinationRepository.ts';
import { ShareActionPolicy } from '../src/domain/service/ShareActionPolicy.ts';
import { DestinationSelectionPolicy } from '../src/domain/service/DestinationSelectionPolicy.ts';
import { ShareTextFormatter } from '../src/domain/service/ShareTextFormatter.ts';
import { UriEncoder } from '../src/domain/service/UriEncoder.ts';
import { UtmParameterPolicy } from '../src/domain/service/UtmParameterPolicy.ts';
import { DraftTemplate } from '../src/domain/service/DraftTemplate.ts';
import { LinkCardPolicy } from '../src/domain/service/LinkCardPolicy.ts';
import type { ShareDestinationDefinition } from '../src/application/ShareButtonCatalog.ts';
import type { ShareSettings } from '../src/application/ShareSettings.ts';

const svg = '<svg><path fill="currentColor"/></svg>';
export const SPECS: readonly ShareDestinationDefinition[] = [
  { key: 'x', label: 'ポスト', color: '#000000', icon: svg, action: ShareAction.Open, endpoint: 'https://twitter.com/intent/tweet', params: { url: 'url', text: 'text', hashtags: 'hashtagsCsv', via: 'via' } },
  { key: 'facebook', label: 'シェア', color: '#1877F2', icon: svg, action: ShareAction.Open, endpoint: 'https://www.facebook.com/sharer/sharer.php', params: { u: 'url' } },
  { key: 'copy', label: 'URLをコピー', color: '#5F6368', icon: svg, action: ShareAction.Copy, endpoint: '', params: {} },
  { key: 'native', label: 'その他', color: '#5F6368', icon: svg, action: ShareAction.Native, endpoint: '', params: {} },
  { key: 'qiita', label: 'Qiita', color: '#55C500', icon: svg, action: ShareAction.Compose, endpoint: 'https://qiita.com/drafts/new', params: {} },
];

export const CONFIG: ShareSettings = {
  destinations: ['facebook', 'x'],
  secondary: ['copy', 'native'],
  labels: { x: 'ポストする' },
  heading: 'SHARE',
  buttons: { copy: { floating: false } },
  appearance: { size: 'small', label_style: 'icon_text', shape: 'official', gap_px: 6, font_family: 'sans-serif', heading_position: 'left', secondary_size_px: 28, secondary_style: 'mono' },
  text: { title_template: '{title} | {site}', hashtags: ['promari'], via: '@promari_jp' },
  utm: { enabled: true, source: '{destination}', medium: 'social', campaign: 'share', content: '' },
  behavior: { open_in_new_tab: true, popup: true, popup_width: 600, popup_height: 500, nofollow: true },
  floating: { position: 'bottom', after: 400, secondaryMax: 1, hideNearEnd: true, destinations: [] },
  tracking: { attribute: 'data-share', event_name: 'promari-sns-share' },
  messages: { copied: 'コピーしました', group_label: 'シェア', composed: '投稿画面に貼り付けてください', compose_failed: 'コピーできませんでした' },
  style: { accent: '#54347e', floating_background: '#fff' },
};

/** ドメインのテストでは、リポジトリのインターフェースを満たす手書きの代役を使う。 */
export const memoryRepository = (specs: readonly ShareDestinationSpec[]): ShareDestinationRepository => {
  const destinations = new Map(specs.map((spec) => [spec.key, new ShareDestination(spec)] as const));
  return {
    has: (key) => destinations.has(key),
    keys: () => [...destinations.keys()],
    resolve: (keys) => keys.map((key) => destinations.get(key) ?? (() => { throw new UnknownShareDestinationError(key); })()),
  };
};

describe('ドメインサービス', () => {
  it('テンプレートを展開する', () => {
    assert.equal(ShareTextFormatter.format('{title} | {site} {url}', { url: 'U', title: 'T', site: 'S' }), 'T | S U');
    // 置換した結果は走査し直さない。題名の {site} は題名のまま残る。
    assert.equal(ShareTextFormatter.format('{title} | {site}', { url: 'U', title: '特集{site}', site: 'S' }), '特集{site} | S');
  });
  it('フラグメントを保って query を足す', () => {
    assert.equal(UriEncoder.appendQuery('https://a.jp/p?x=1#h', { a: 'b c' }), 'https://a.jp/p?x=1&a=b%20c#h');
    assert.equal(UriEncoder.appendQuery('https://a.jp/p', { a: '1' }), 'https://a.jp/p?a=1');
    // encodeURIComponent が残す ! ' ( ) * も、RFC 3986 の予約文字なので符号化する。
    assert.equal(UriEncoder.appendQuery('https://a.jp/p', { 'k!': "v'(*)" }), 'https://a.jp/p?k%21=v%27%28%2A%29');
  });
  it('mailto と下書きを送る共有先は小窓で開かない', () => {
    assert.equal(ShareActionPolicy.canOpenInPopup({ href: 'mailto:someone@example.com?subject=x', sendsDraft: false }), false);
    assert.equal(ShareActionPolicy.canOpenInPopup({ href: 'https://a.jp/', sendsDraft: false }), true);
    // 下書きを送る共有先はサービスの投稿画面を開くので、小窓にしない（ADR-0004）。
    assert.equal(ShareActionPolicy.canOpenInPopup({ href: 'https://a.jp/', sendsDraft: true }), false);
  });
  it('utm は enabled のときだけ・空の項目は付けない・{destination} を置換する', () => {
    assert.equal(UtmParameterPolicy.apply('https://a.jp/', { ...CONFIG.utm, enabled: false }, 'x'), 'https://a.jp/');
    assert.equal(UtmParameterPolicy.apply('https://a.jp/', CONFIG.utm, 'x'), 'https://a.jp/?utm_source=x&utm_medium=social&utm_campaign=share');
  });
  it('native は端末が対応するときだけ・固定バーは floating=false を除き最大数で切る', () => {
    const repository = memoryRepository(SPECS);
    assert.deepEqual(DestinationSelectionPolicy.select(CONFIG, { placement: 'inline', canNativeShare: false, repository }).secondary, ['copy']);
    assert.deepEqual(DestinationSelectionPolicy.select(CONFIG, { placement: 'inline', canNativeShare: true, repository }).secondary, ['copy', 'native']);
    assert.deepEqual(DestinationSelectionPolicy.select(CONFIG, { placement: 'floating', canNativeShare: true, repository }).secondary, ['native']);
    assert.deepEqual(DestinationSelectionPolicy.select({ ...CONFIG, floating: { ...CONFIG.floating, destinations: ['x'] } }, { placement: 'floating', canNativeShare: true, repository }).primary, ['x']);
  });
  it('クリックの判断', () => {
    assert.equal(ShareActionPolicy.decide({ action: ShareAction.Copy, popup: null }), 'copy');
    assert.equal(ShareActionPolicy.decide({ action: ShareAction.Native, popup: null }), 'native');
    assert.equal(ShareActionPolicy.decide({ action: ShareAction.Open, popup: { width: 1, height: 1 } }), 'popup');
    assert.equal(ShareActionPolicy.decide({ action: ShareAction.Open, popup: null }), 'follow');
    // compose は小窓の設定に関わらず compose（常に新しいタブ）。
    assert.equal(ShareActionPolicy.decide({ action: ShareAction.Compose, popup: { width: 1, height: 1 } }), 'compose');
    assert.equal(ShareActionPolicy.decide({ action: ShareAction.Compose, popup: null }), 'compose');
  });
  it('操作と href の関係は domain の1か所で決める（リンクになれるか・リンクをたどること自体が操作か）', () => {
    const table = Object.values(ShareAction).map((action) => [action, ShareActionPolicy.followsLink(action), ShareActionPolicy.linkable(action)]);
    assert.deepEqual(table, [
      ['open', true, true],
      ['copy', false, true], // href はページ自身。既定の共有欄では従来どおりリンク
      ['native', false, true],
      ['compose', false, false], // href だけをたどるとコピーが抜けるので、リンクにしない
    ]);
  });
});

describe('ShareDestination（値オブジェクト）', () => {
  it('ドメインの共有先へ表示用メタデータを持ち込まない', () => {
    const destination = new ShareDestination(SPECS[0]!);
    assert.deepEqual(Object.keys(destination).sort(), ['action', 'key']);
    assert.equal('appearance' in destination, false);
    assert.ok(Object.isFrozen(destination));
  });
  it('RFC 3986 でエンコードし、空の値は送らない', () => {
    const x = new ShareDestination(SPECS[0]!);
    const request = ShareRequest.create({ url: 'https://a.jp/?q=1', title: 'T', text: 'a b+c', hashtags: ['p', 'q'], via: '@v' });
    assert.equal(x!.shareUrl(request), 'https://twitter.com/intent/tweet?url=https%3A%2F%2Fa.jp%2F%3Fq%3D1&text=a%20b%2Bc&hashtags=p%2Cq&via=v');
    const empty = ShareRequest.create({ url: 'https://a.jp/', title: '', text: '' });
    assert.equal(x!.shareUrl(empty), 'https://twitter.com/intent/tweet?url=https%3A%2F%2Fa.jp%2F');
    // encodeURIComponent は ! ' ( ) * を残すので、RFC 3986 に合わせて補って符号化する。
    const tricky = ShareRequest.create({ url: 'https://a.jp/', title: '', text: "a!b'c(d)e*f" });
    assert.equal(x!.shareUrl(tricky), 'https://twitter.com/intent/tweet?url=https%3A%2F%2Fa.jp%2F&text=a%21b%27c%28d%29e%2Af');
  });
  it('compose の共有先は投稿画面の URL をそのまま返す（URL に項目を載せない）', () => {
    const qiita = new ShareDestination(SPECS[4]!);
    assert.equal(qiita.shareUrl(ShareRequest.create({ url: 'https://a.jp/', title: 'T', text: 'T' })), 'https://qiita.com/drafts/new');
  });
  it('compose の下書きは「題名＋改行＋URL」、題名が空なら URL だけ。compose 以外は空', () => {
    const qiita = new ShareDestination(SPECS[4]!);
    assert.equal(qiita.composeDraft(ShareRequest.create({ url: 'https://a.jp/', title: ' Hello ', text: '' })), 'Hello\nhttps://a.jp/');
    assert.equal(qiita.composeDraft(ShareRequest.create({ url: 'https://a.jp/', title: '  ', text: '' })), 'https://a.jp/');
    for (const spec of SPECS.slice(0, 4)) assert.equal(new ShareDestination(spec).composeDraft(ShareRequest.create({ url: 'https://a.jp/', title: 'T', text: 'T' })), '', spec.key);
  });
  it('endpoint の無いサービスはページ URL を返す', () => {
    const copy = new ShareDestination(SPECS[2]!);
    assert.equal(copy!.shareUrl(ShareRequest.create({ url: 'https://a.jp/', title: '', text: '' })), 'https://a.jp/');
  });
});

/** A fictional service whose editor reads a title and an HTML card from the query, like Ameba Blog (ADR-0004). */
export const CARD_SPEC: ShareDestinationDefinition = {
  key: 'cardblog', label: 'Card', color: '#123456', icon: svg, action: ShareAction.Open, endpoint: 'https://blog.example/new',
  params: { t: 'title', body: 'draft' },
  draft: { template: '<a href="{url}"><b>{title}</b><i>{description}</i><s>{host}</s><img src="{image}"></a>', format: 'html' },
};
const request = (input: Partial<Parameters<typeof ShareRequest.create>[0]> = {}): ShareRequest =>
  ShareRequest.create({ url: 'https://a.jp/p?x=1&y=2', title: 'T', text: 'T', ...input });

describe('DraftTemplate（下書きのひな形）', () => {
  it('text は値をそのまま差し込み、先頭の空行を残さない', () => {
    const qiita = { template: '{title}\n\n{url}\n', format: 'text' } as const;
    assert.equal(DraftTemplate.render(qiita, request({ title: ' Hello ' })), 'Hello\n\nhttps://a.jp/p?x=1&y=2\n');
    assert.equal(DraftTemplate.render(qiita, request({ title: '' })), 'https://a.jp/p?x=1&y=2\n');
    assert.equal(DraftTemplate.render({ template: '{title}\n\n@[card]({url})\n', format: 'text' }, request({ title: 'A & B' })), 'A & B\n\n@[card](https://a.jp/p?x=1&y=2)\n');
  });
  it('html は値ごとに HTML エスケープする（ひな形そのものは変えない）', () => {
    const html = DraftTemplate.render(CARD_SPEC.draft!, request({ title: '<script>"x" & \'y\'', description: 'a<b', image: 'https://a.jp/i.png?a=1&b=2' }));
    assert.equal(html, '<a href="https://a.jp/p?x=1&amp;y=2"><b>&lt;script&gt;&quot;x&quot; &amp; &#39;y&#39;</b><i>a&lt;b</i><s>a.jp</s><img src="https://a.jp/i.png?a=1&amp;b=2"></a>');
  });
  it('一度の走査で置き換え、差し込んだ値の中の差し込み口は展開しない', () => {
    assert.equal(DraftTemplate.render({ template: '{title} {url}', format: 'text' }, request({ title: '{url}' })), '{url} https://a.jp/p?x=1&y=2');
  });
  it('{host} は URL の資格情報とポートを除いたホスト名', () => {
    assert.equal(DraftTemplate.host('https://user:pw@example.com:8443/a?b#c'), 'example.com');
    assert.equal(DraftTemplate.host('https://[::1]:8080/'), '[::1]');
    assert.equal(DraftTemplate.host('/relative'), '');
  });
});

describe('LinkCardPolicy（URL の長さの上限）', () => {
  it('clip は文字（コードポイント）単位で切り、切ったら末尾を「…」にする', () => {
    assert.equal(LinkCardPolicy.clip('abc', 3), 'abc');
    assert.equal(LinkCardPolicy.clip('abcd', 3), 'ab…');
    assert.equal(LinkCardPolicy.clip('😀😀😀😀', 3), '😀😀…'); // サロゲートペアを割らない
    assert.equal(LinkCardPolicy.clip('abc', 0), '');
  });
  it('bound は題名を100文字、説明を60文字に切り、改行などの空白を詰める', () => {
    const bounded = LinkCardPolicy.bound(request({ title: 'あ'.repeat(120), description: '説明\n  の\t文'.padEnd(80, 'い') }));
    assert.equal(Array.from(bounded.title).length, 100);
    assert.ok(bounded.title.endsWith('…'));
    assert.equal(Array.from(bounded.description).length, 60);
    assert.ok(bounded.description.startsWith('説明 の 文'));
    assert.equal(LinkCardPolicy.bound(request({ description: '短い' })).description, '短い');
  });
  it('上限内ならそのまま、超えたら説明を縮めて外し、次に画像を外し、最後に題名を縮める', () => {
    const build = (r: ShareRequest): string => `${r.title}|${r.description}|${r.image}`;
    const full = request({ title: 'T'.repeat(10), description: 'D'.repeat(10), image: 'I'.repeat(10) });
    assert.equal(LinkCardPolicy.fit(full, build, 100), `${'T'.repeat(10)}|${'D'.repeat(10)}|${'I'.repeat(10)}`);
    // 説明を縮めれば収まる: 画像と題名は残す。
    assert.equal(LinkCardPolicy.fit(full, build, 27), `${'T'.repeat(10)}|${'D'.repeat(4)}…|${'I'.repeat(10)}`);
    // 説明を外しても収まらない: 画像を外す（説明は空のまま）。
    assert.equal(LinkCardPolicy.fit(full, build, 15), `${'T'.repeat(10)}||`);
    // 画像を外しても収まらない: 題名を縮める。
    assert.equal(LinkCardPolicy.fit(full, build, 8), `${'T'.repeat(5)}…||`);
    // 何を外しても収まらない（ページの URL だけで超える）: いちばん短い形を返す。
    assert.equal(LinkCardPolicy.fit(full, build, 1), '||');
  });
});

describe('ShareDestination（下書きを持つ共有先）', () => {
  it('open の共有先は params で題名と下書き（HTML のカード）を送る', () => {
    const card = new ShareDestination(CARD_SPEC);
    const href = card.shareUrl(request({ title: 'A&B', description: 'desc', image: 'https://a.jp/i.png' }));
    const query = new URLSearchParams(href.slice(href.indexOf('?') + 1));
    assert.ok(href.startsWith('https://blog.example/new?t='));
    assert.equal(query.get('t'), 'A&B');
    assert.equal(query.get('body'), '<a href="https://a.jp/p?x=1&amp;y=2"><b>A&amp;B</b><i>desc</i><s>a.jp</s><img src="https://a.jp/i.png"></a>');
    assert.equal(card.composeDraft(request()), '', 'open はコピーしない');
    assert.equal(new ShareDestination(SPECS[0]!).sendsDraft, false);
  });
  it('下書きを送る URL は 3,500 文字以内に収める（日本語の長い題名でも）', () => {
    const card = new ShareDestination({ ...CARD_SPEC, draft: { template: `${'<p style="x">'.repeat(60)}{title}{description}{image}`, format: 'html' } });
    for (const title of ['あ'.repeat(40), 'あ'.repeat(100), 'あ'.repeat(300)]) {
      const href = card.shareUrl(request({ title, description: 'い'.repeat(60), image: `https://a.jp/${'i'.repeat(200)}.png` }));
      assert.ok(href.length <= LinkCardPolicy.URL_MAX, `${Array.from(title).length}: ${href.length}`);
      assert.match(decodeURIComponent(href), /あ/, '題名は残す');
    }
  });
  it('compose の共有先は宣言した下書きをコピーし、題名・説明は上限で切る', () => {
    const qiita = new ShareDestination({ ...SPECS[4]!, draft: { template: '{title}\n\n{url}\n', format: 'text' } });
    assert.equal(qiita.composeDraft(request({ title: 'Hello' })), 'Hello\n\nhttps://a.jp/p?x=1&y=2\n');
    assert.equal(qiita.composeDraft(request({ title: 'あ'.repeat(150) })), `${'あ'.repeat(99)}…\n\nhttps://a.jp/p?x=1&y=2\n`);
    const medium = new ShareDestination({ ...SPECS[4]!, draft: { template: '{url}', format: 'text' } });
    assert.equal(medium.composeDraft(request({ title: 'Hello' })), 'https://a.jp/p?x=1&y=2');
    assert.equal(qiita.shareUrl(request()), 'https://qiita.com/drafts/new', 'compose は URL に何も載せない');
  });
});
