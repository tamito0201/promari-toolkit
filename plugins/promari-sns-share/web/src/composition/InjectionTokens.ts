/**
 * Typed keys for the DI container. Each key carries the type it resolves to, so a binding and
 * a lookup cannot disagree about the type. Only the composition code uses these keys; the four
 * layers receive their dependencies through constructors and never see the container.
 */
import type { BuildShareBarUseCase } from '../application/BuildShareBarUseCase.ts';
import type { HandleShareClickUseCase } from '../application/HandleShareClickUseCase.ts';
import type { ShareButtonCatalog, ShareDestinationDefinition } from '../application/ShareButtonCatalog.ts';
import type { ShareSettings } from '../application/ShareSettings.ts';
import type { ClipboardGateway } from '../domain/gateway/ClipboardGateway.ts';
import type { ShareActivityPublisher } from '../domain/gateway/ShareActivityPublisher.ts';
import type { NativeShareGateway } from '../domain/gateway/NativeShareGateway.ts';
import type { ShareWindowGateway } from '../domain/gateway/ShareWindowGateway.ts';
import type { SharedPageGateway } from '../domain/gateway/SharedPageGateway.ts';
import type { ShareDestinationRepository } from '../domain/repository/ShareDestinationRepository.ts';
import type { ShareElementDependencies } from '../presentation/PromariSnsShareElement.ts';

/** A symbol that remembers the type bound to it. The type exists only at compile time. */
export type InjectionToken<T> = symbol & { readonly __resolves?: T };

const token = <T>(description: string): InjectionToken<T> => Symbol(description) as InjectionToken<T>;

/** How activity events are published for one element. Browsers dispatch a CustomEvent on the element. */
export type ShareActivityPublisherFactory = (element: HTMLElement, eventName: string) => ShareActivityPublisher;

/** Creates the click use case for one element. Activity events go to that element under the given name. */
export type HandleShareClickUseCaseFactory = (element: HTMLElement, eventName: string) => HandleShareClickUseCase;

export const TOKENS = Object.freeze({
  // Generated input from tools/config.py.
  ShareDestinationDefinitions: token<readonly ShareDestinationDefinition[]>('ShareDestinationDefinitions'),
  DefaultShareSettings: token<ShareSettings>('DefaultShareSettings'),
  // domain interfaces, implemented in infrastructure.
  ShareDestinationRepository: token<ShareDestinationRepository>('ShareDestinationRepository'),
  SharedPageGateway: token<SharedPageGateway>('SharedPageGateway'),
  NativeShareGateway: token<NativeShareGateway>('NativeShareGateway'),
  ClipboardGateway: token<ClipboardGateway>('ClipboardGateway'),
  ShareWindowGateway: token<ShareWindowGateway>('ShareWindowGateway'),
  ShareActivityPublisherFactory: token<ShareActivityPublisherFactory>('ShareActivityPublisherFactory'),
  // application.
  ShareButtonCatalog: token<ShareButtonCatalog>('ShareButtonCatalog'),
  BuildShareBarUseCase: token<BuildShareBarUseCase>('BuildShareBarUseCase'),
  HandleShareClickUseCaseFactory: token<HandleShareClickUseCaseFactory>('HandleShareClickUseCaseFactory'),
  // presentation.
  ShareElementDependencies: token<ShareElementDependencies>('ShareElementDependencies'),
});

/** The token types, in order. Used so that a factory's parameters follow the tokens it lists. */
export type InjectionTokens<A extends readonly unknown[]> = { readonly [K in keyof A]: InjectionToken<A[K]> };
