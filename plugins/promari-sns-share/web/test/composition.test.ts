/**
 * The DI container (InversifyJS) built in composition/. The four layers are tested with plain
 * objects elsewhere; here we check what the container wires together.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { ContainerModule } from 'inversify';
import { ShareContainer } from '../src/composition/ShareContainer.ts';
import { TOKENS } from '../src/composition/InjectionTokens.ts';
import { TypedBinding } from '../src/composition/TypedBinding.ts';
import { CATALOG } from '../src/generated/catalog.ts';
import { DEFAULTS } from '../src/generated/defaults.ts';
import { InMemoryShareDestinationRepository } from '../src/infrastructure/InMemoryShareDestinationRepository.ts';
import { BrowserClipboard } from '../src/infrastructure/BrowserClipboard.ts';
import { BrowserNativeShare } from '../src/infrastructure/BrowserNativeShare.ts';
import { BrowserPopupWindow } from '../src/infrastructure/BrowserPopupWindow.ts';
import { BrowserSharedPage } from '../src/infrastructure/BrowserSharedPage.ts';
import type { ShareActivity } from '../src/domain/gateway/ShareActivityPublisher.ts';

/** Replaces the browser with recording fakes, keeping the real repository. */
const fakeInfrastructure = (calls: string[], published: Array<{ element: string; eventName: string; activity: ShareActivity }>) =>
  new ContainerModule(({ bind }) => {
    const { provide, constant } = TypedBinding;
    provide(bind, TOKENS.ShareDestinationRepository, [TOKENS.ShareDestinationDefinitions], definitions => new InMemoryShareDestinationRepository(definitions));
    constant(bind, TOKENS.SharedPageGateway, { read: () => ({ url: 'https://a.jp/post/', title: 'Hello', site: 'Promari' }) });
    constant(bind, TOKENS.NativeShareGateway, { available: true, share: async d => { calls.push(`share:${d.url}`); } });
    constant(bind, TOKENS.ClipboardGateway, { write: async t => { calls.push(`copy:${t}`); }, fallback: t => { calls.push(`fallback:${t}`); } });
    constant(bind, TOKENS.ShareWindowGateway, { open: href => { calls.push(`popup:${href}`); return true; } });
    constant(bind, TOKENS.ShareActivityPublisherFactory, (element, eventName) => ({
      publish: activity => { published.push({ element: element.id, eventName, activity }); },
    }));
  });

const element = (id: string) => ({ id }) as unknown as HTMLElement;

describe('ShareContainer', () => {
  it('1ページに1つずつ作り、同じトークンには同じインスタンスを返す', () => {
    const container = ShareContainer.create(CATALOG, DEFAULTS, fakeInfrastructure([], []));
    assert.equal(ShareContainer.get(container, TOKENS.BuildShareBarUseCase), ShareContainer.get(container, TOKENS.BuildShareBarUseCase));
    assert.equal(ShareContainer.get(container, TOKENS.ShareElementDependencies).defaults, DEFAULTS);
  });

  it('独自要素へ渡す依存から、共有欄を組み立てられる', () => {
    const deps = ShareContainer.get(ShareContainer.create(CATALOG, DEFAULTS, fakeInfrastructure([], [])), TOKENS.ShareElementDependencies);
    const environment = deps.connect(element('a'));
    const page = environment.pageContext();
    const bar = deps.buildShareBar.execute(DEFAULTS, { ...page, placement: 'inline', canNativeShare: environment.canNativeShare() });
    assert.deepEqual(bar.primary.map(button => button.key), DEFAULTS.destinations);
  });

  it('クリックの通知は要素ごと・イベント名ごとに分かれる', async () => {
    const calls: string[] = [];
    const published: Array<{ element: string; eventName: string; activity: ShareActivity }> = [];
    const deps = ShareContainer.get(ShareContainer.create(CATALOG, DEFAULTS, fakeInfrastructure(calls, published)), TOKENS.ShareElementDependencies);
    const bar = deps.buildShareBar.execute({ ...DEFAULTS, secondary: ['copy'] }, { url: 'https://a.jp/post/', title: 'Hello', site: 'Promari', placement: 'inline', canNativeShare: false });
    const copy = bar.secondary.find(button => button.key === 'copy')!;
    const top = deps.connect(element('top')).clickUseCase('share-top');
    const bottom = deps.connect(element('bottom')).clickUseCase('share-bottom');
    assert.notEqual(top, bottom);
    assert.equal(await top.execute({ button: copy, placement: 'article_top', preventDefault: () => undefined }), 'copied');
    await bottom.execute({ button: copy, placement: 'article_bottom', preventDefault: () => undefined });
    assert.deepEqual(calls, ['copy:https://a.jp/post/', 'copy:https://a.jp/post/']);
    assert.deepEqual(published.map(p => [p.element, p.eventName, p.activity.placement]),
      [['top', 'share-top', 'article_top'], ['bottom', 'share-bottom', 'article_bottom']]);
  });

  it('ブラウザ用のモジュールは、domainのインターフェースへブラウザ実装を結び付ける', () => {
    const container = ShareContainer.create(CATALOG, DEFAULTS);
    assert.ok(ShareContainer.get(container, TOKENS.ShareDestinationRepository) instanceof InMemoryShareDestinationRepository);
    assert.ok(ShareContainer.get(container, TOKENS.SharedPageGateway) instanceof BrowserSharedPage);
    assert.ok(ShareContainer.get(container, TOKENS.NativeShareGateway) instanceof BrowserNativeShare);
    assert.ok(ShareContainer.get(container, TOKENS.ClipboardGateway) instanceof BrowserClipboard);
    assert.ok(ShareContainer.get(container, TOKENS.ShareWindowGateway) instanceof BrowserPopupWindow);
  });

  it('結び付けていないトークンは解決時に失敗する（黙って undefined を返さない）', () => {
    const container = ShareContainer.create(CATALOG, DEFAULTS, new ContainerModule(() => undefined));
    assert.throws(() => ShareContainer.get(container, TOKENS.ShareElementDependencies));
  });
});
