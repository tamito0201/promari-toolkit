/**
 * Builds the DI container (InversifyJS). This is the only code that knows both the domain
 * interfaces and the browser implementations. Bindings use factories rather than decorators,
 * so the four layers stay free of container code and run unchanged in tests.
 *
 * Modules:
 * - generated input: destination definitions and default settings from tools/config.py
 * - browser infrastructure: the implementations of the domain gateways and repository
 * - share application: use cases and what the custom element receives
 * Tests replace the infrastructure module with fakes and keep the other two.
 */
import { Container, ContainerModule } from 'inversify';
import { BuildShareBarUseCase } from '../application/BuildShareBarUseCase.ts';
import { HandleShareClickUseCase } from '../application/HandleShareClickUseCase.ts';
import { ShareButtonCatalog, type ShareDestinationDefinition } from '../application/ShareButtonCatalog.ts';
import type { ShareSettings } from '../application/ShareSettings.ts';
import { BrowserClipboard } from '../infrastructure/BrowserClipboard.ts';
import { BrowserNativeShare } from '../infrastructure/BrowserNativeShare.ts';
import { BrowserPopupWindow } from '../infrastructure/BrowserPopupWindow.ts';
import { BrowserSharedPage } from '../infrastructure/BrowserSharedPage.ts';
import { CustomEventShareActivityPublisher } from '../infrastructure/CustomEventShareActivityPublisher.ts';
import { InMemoryShareDestinationRepository } from '../infrastructure/InMemoryShareDestinationRepository.ts';
import {
  TOKENS, type InjectionToken, type HandleShareClickUseCaseFactory, type ShareActivityPublisherFactory,
} from './InjectionTokens.ts';
import { TypedBinding } from './TypedBinding.ts';

const { provide, constant } = TypedBinding;

export class ShareContainer {
  /** Generated data that the other modules read. */
  static generatedInput(definitions: readonly ShareDestinationDefinition[], defaults: ShareSettings): ContainerModule {
    return new ContainerModule(({ bind }) => {
      constant(bind, TOKENS.ShareDestinationDefinitions, definitions);
      constant(bind, TOKENS.DefaultShareSettings, defaults);
    });
  }

  /** Browser implementations of the domain interfaces. */
  static browserInfrastructure(
    publisher: ShareActivityPublisherFactory = (element, eventName) => new CustomEventShareActivityPublisher(element, eventName),
  ): ContainerModule {
    return new ContainerModule(({ bind }) => {
      provide(bind, TOKENS.ShareDestinationRepository, [TOKENS.ShareDestinationDefinitions],
        definitions => new InMemoryShareDestinationRepository(definitions));
      provide(bind, TOKENS.SharedPageGateway, [], () => new BrowserSharedPage());
      provide(bind, TOKENS.NativeShareGateway, [], () => new BrowserNativeShare());
      provide(bind, TOKENS.ClipboardGateway, [], () => new BrowserClipboard());
      provide(bind, TOKENS.ShareWindowGateway, [], () => new BrowserPopupWindow());
      constant(bind, TOKENS.ShareActivityPublisherFactory, publisher);
    });
  }

  /** Use cases and the dependencies handed to the custom element. */
  static shareApplication(): ContainerModule {
    return new ContainerModule(({ bind }) => {
      provide(bind, TOKENS.ShareButtonCatalog, [TOKENS.ShareDestinationRepository, TOKENS.ShareDestinationDefinitions],
        (repository, definitions) => new ShareButtonCatalog(repository, definitions));
      provide(bind, TOKENS.BuildShareBarUseCase, [TOKENS.ShareButtonCatalog], catalog => new BuildShareBarUseCase(catalog));
      // Each element publishes activity under its own name, so the click use case is made per element.
      provide(bind, TOKENS.HandleShareClickUseCaseFactory,
        [TOKENS.ClipboardGateway, TOKENS.NativeShareGateway, TOKENS.ShareWindowGateway, TOKENS.ShareActivityPublisherFactory],
        (clipboard, nativeShare, popup, publish): HandleShareClickUseCaseFactory => (element, eventName) =>
          new HandleShareClickUseCase({ clipboard, nativeShare, popup, activity: publish(element, eventName) }));
      provide(bind, TOKENS.ShareElementDependencies,
        [TOKENS.BuildShareBarUseCase, TOKENS.DefaultShareSettings, TOKENS.SharedPageGateway, TOKENS.NativeShareGateway,
          TOKENS.HandleShareClickUseCaseFactory],
        (buildShareBar, defaults, page, nativeShare, clickUseCase) => ({
          buildShareBar,
          defaults,
          connect: element => ({
            pageContext: () => page.read(),
            canNativeShare: () => nativeShare.available,
            clickUseCase: eventName => clickUseCase(element, eventName),
          }),
        }));
    });
  }

  /**
   * The container used in browsers. Every binding is a singleton: one instance per page.
   * Pass another infrastructure module to run without a browser.
   */
  static create(
    definitions: readonly ShareDestinationDefinition[],
    defaults: ShareSettings,
    infrastructure: ContainerModule = ShareContainer.browserInfrastructure(),
  ): Container {
    const container = new Container({ defaultScope: 'Singleton' });
    container.load(ShareContainer.generatedInput(definitions, defaults), infrastructure, ShareContainer.shareApplication());
    return container;
  }

  /** Look up a token with its bound type. */
  static get<T>(container: Container, token: InjectionToken<T>): T {
    return container.get<T>(token);
  }
}
