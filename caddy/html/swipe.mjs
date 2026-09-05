// One day per deliberate horizontal gesture. Browser retains vertical pan/zoom.
export function bindDaySwipe(element, moveDay) {
  let start = null;
  const reset = () => { start = null; };
  element.addEventListener('pointerdown', event => {
    if (!event.isPrimary) { reset(); return; }
    if (event.button !== 0) return;
    start = { id: event.pointerId, x: event.clientX, y: event.clientY };
    element.setPointerCapture(event.pointerId);
  });
  element.addEventListener('pointermove', event => {
    if (!start || event.pointerId !== start.id) return;
    // Once a vertical scroll is apparent, don't turn its tail into a swipe.
    const dx = Math.abs(event.clientX - start.x);
    const dy = Math.abs(event.clientY - start.y);
    if (dy > 12 && dy > dx) reset();
  });
  element.addEventListener('pointerup', event => {
    if (!start || event.pointerId !== start.id) return;
    const dx = event.clientX - start.x;
    const dy = event.clientY - start.y;
    reset();
    if (Math.abs(dx) >= 40 && Math.abs(dx) > Math.abs(dy) * 1.5) {
      moveDay(dx < 0 ? 1 : -1);
    }
  });
  element.addEventListener('pointercancel', reset);
  element.addEventListener('lostpointercapture', reset);
  element.addEventListener('keydown', event => {
    if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
    event.preventDefault();
    moveDay(event.key === 'ArrowRight' ? 1 : -1);
  });
}
