/**
 * Show the floating bar after scrolling and hide it near the end of the page.
 * This is display behavior of the element, so it lives in presentation.
 */
export class FloatingVisibility {
  readonly #element: HTMLElement;
  readonly #after: number;
  readonly #hideNearEnd: boolean;
  #ticking = false;
  readonly #onScroll = (): void => { if (!this.#ticking) { this.#ticking = true; requestAnimationFrame(() => this.#update()); } };

  constructor(element: HTMLElement, { after, hideNearEnd }: { after: number; hideNearEnd: boolean }) {
    this.#element = element;
    this.#after = after;
    this.#hideNearEnd = hideNearEnd;
  }

  start(): void {
    window.addEventListener('scroll', this.#onScroll, { passive: true });
    this.#update();
  }

  stop(): void {
    window.removeEventListener('scroll', this.#onScroll);
  }

  #update(): void {
    const y = window.scrollY;
    const end = document.documentElement.scrollHeight - window.innerHeight - 120;
    this.#element.classList.toggle('on', y > this.#after && (!this.#hideNearEnd || y < end));
    this.#ticking = false;
  }
}
