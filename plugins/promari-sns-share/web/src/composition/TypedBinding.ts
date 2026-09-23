/**
 * Type-safe binding helpers for InversifyJS. A token carries the type it resolves to, so a
 * value or factory that does not match the token, or dependencies listed in the wrong order,
 * are compile errors instead of wrong objects at runtime.
 */
import type { Bind, MapToResolvedValueInjectOptions } from 'inversify';
import type { InjectionToken, InjectionTokens } from './InjectionTokens.ts';

export class TypedBinding {
  /** Bind a token to a factory whose parameters are the listed tokens, in order. */
  static provide<T, A extends unknown[]>(bind: Bind, token: InjectionToken<T>, dependencies: InjectionTokens<A>, factory: (...args: A) => T): void {
    bind<T>(token).toResolvedValue(factory, [...dependencies] as unknown as MapToResolvedValueInjectOptions<A>);
  }

  /** Bind a token to a value that already exists. */
  static constant<T>(bind: Bind, token: InjectionToken<T>, value: T): void {
    bind<T>(token).toConstantValue(value);
  }
}
