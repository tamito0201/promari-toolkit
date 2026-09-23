/** 画面内の通知。操作の結果を受け取った表示層が、押された要素の識別子とともに呼ぶ。 */
export interface Notifier {
  notify(text: string, target?: string): void;
}

/**
 * 表示領域内の通知を描く。
 */
export const toastNotifier = (root: ShadowRoot): Notifier => {
  let toast: HTMLDivElement | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  return {
    notify: (text, target) => {
      const anchor = target === undefined ? null : root.querySelector(`[data-notification-target="${CSS.escape(target)}"]`);
      const status = root.querySelector('.status');
      if (status) { status.textContent = text; return; }
      toast ??= Object.assign(root.appendChild(document.createElement('div')), { className: 't' });
      toast.setAttribute('role', 'status');
      toast.textContent = text;
      toast.classList.add('on');
      anchor?.classList.add('done');
      clearTimeout(timer);
      timer = setTimeout(() => { toast?.classList.remove('on'); anchor?.classList.remove('done'); }, 1800);
    },
  };
};

