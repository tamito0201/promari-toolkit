/**
 * Implements ShareActivityPublisher by dispatching a bubbling, composed CustomEvent from the
 * host element. No telemetry request is sent; the host page decides what to do with it.
 */
import type { ShareActivity, ShareActivityPublisher } from '../domain/gateway/ShareActivityPublisher.ts';

export class CustomEventShareActivityPublisher implements ShareActivityPublisher {
  readonly #element: HTMLElement;
  readonly #eventName: string;

  constructor(element: HTMLElement, eventName: string) {
    this.#element = element;
    this.#eventName = eventName;
  }

  publish(activity: ShareActivity): void {
    this.#element.dispatchEvent(new CustomEvent<ShareActivity>(this.#eventName, { bubbles: true, composed: true, detail: activity }));
  }
}
