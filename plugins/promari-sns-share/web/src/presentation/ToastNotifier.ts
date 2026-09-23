/**
 * 画面内の通知。操作の結果を受け取った表示層が、押された要素の識別子とともに呼ぶ。
 */
export class ToastNotifier {
  readonly #root: ShadowRoot;
  #toast: HTMLDivElement | undefined;
  #timer: ReturnType<typeof setTimeout> | undefined;

  constructor(root: ShadowRoot) {
    this.#root = root;
  }

  notify(text: string, target?: string): void {
    const anchor = target === undefined ? null : this.#root.querySelector(`[data-notification-target="${CSS.escape(target)}"]`);
    const status = this.#root.querySelector('.status');
    if (status) { status.textContent = text; return; }
    this.#toast ??= Object.assign(this.#root.appendChild(document.createElement('div')), { className: 't' });
    const toast = this.#toast;
    toast.setAttribute('role', 'status');
    toast.textContent = text;
    toast.classList.add('on');
    anchor?.classList.add('done');
    clearTimeout(this.#timer);
    this.#timer = setTimeout(() => { toast.classList.remove('on'); anchor?.classList.remove('done'); }, 1800);
  }
}
