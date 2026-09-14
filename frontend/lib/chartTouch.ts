import type { TouchEvent as ReactTouchEvent } from 'react';

// RechartsWrapper.js only ever routes `touchstart` to an external handler —
// it never reaches the library's own tooltip state, which is written from
// `touchmove` via `touchEventsMiddleware` (`setMouseOverAxisIndex`). Without
// this, a tap or a press held still shows nothing: the tooltip only appears
// once the finger has already moved. Re-dispatching the touchstart's own
// touch points as a synthetic, bubbling `touchmove` on the touched element
// feeds that same middleware on first contact, using no API beyond the
// `TouchEvent` constructor Recharts itself listens for.
export function replayTouchAsMove(event: ReactTouchEvent): boolean {
  if (!(event.target instanceof Element)) return false;
  if (event.touches.length === 0) return false;
  if (typeof TouchEvent !== 'function') return false;

  try {
    // event.nativeEvent's touch lists carry the underlying native `Touch`
    // objects the `TouchEvent` constructor requires — React's own `Touch`
    // type is a narrower synthetic wrapper that TypeScript won't accept here.
    const native = event.nativeEvent;
    const touchmove = new TouchEvent('touchmove', {
      bubbles: true,
      cancelable: true,
      touches: Array.from(native.touches),
      targetTouches: Array.from(native.targetTouches),
      changedTouches: Array.from(native.changedTouches),
    });
    event.target.dispatchEvent(touchmove);
    return true;
  } catch {
    // The TouchEvent constructor isn't available in every browser — fall
    // back to Recharts' own touchmove handling once the finger moves.
    return false;
  }
}
