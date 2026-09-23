/**
 * Show the floating bar after scrolling and hide it near the end of the page.
 * This is display behavior of the element, so it lives in presentation. Return a cleanup callback.
 */
export const watchFloatingVisibility = (element: HTMLElement, { after, hideNearEnd }: { after: number; hideNearEnd: boolean }): (() => void) => {
  let ticking = false;
  const update = (): void => {
    const y = window.scrollY;
    const end = document.documentElement.scrollHeight - window.innerHeight - 120;
    element.classList.toggle('on', y > after && (!hideNearEnd || y < end));
    ticking = false;
  };
  const onScroll = (): void => { if (!ticking) { ticking = true; requestAnimationFrame(update); } };
  window.addEventListener('scroll', onScroll, { passive: true });
  update();
  return () => window.removeEventListener('scroll', onScroll);
};
