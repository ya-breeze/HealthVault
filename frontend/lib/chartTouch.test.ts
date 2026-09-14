import { beforeAll, describe, expect, it, vi } from 'vitest';
import { replayTouchAsMove } from './chartTouch';

class FakeTouch {
  identifier: number;
  clientX: number;
  clientY: number;
  constructor(init: { identifier: number; clientX: number; clientY: number }) {
    this.identifier = init.identifier;
    this.clientX = init.clientX;
    this.clientY = init.clientY;
  }
}

class FakeTouchEvent extends Event {
  touches: FakeTouch[];
  targetTouches: FakeTouch[];
  changedTouches: FakeTouch[];
  constructor(
    type: string,
    init: EventInit & { touches: FakeTouch[]; targetTouches: FakeTouch[]; changedTouches: FakeTouch[] }
  ) {
    super(type, init);
    this.touches = init.touches;
    this.targetTouches = init.targetTouches;
    this.changedTouches = init.changedTouches;
  }
}

// This suite runs under Vitest's 'node' environment (vitest.config.mts), which
// has no DOM. replayTouchAsMove only needs `Element` (for its instanceof
// guard) and the `TouchEvent` constructor it feature-detects — Node already
// provides `Event`/`EventTarget` natively, so these minimal stand-ins are
// enough without pulling in jsdom for one file.
beforeAll(() => {
  (globalThis as unknown as { Element: unknown }).Element = class extends EventTarget {};
  (globalThis as unknown as { TouchEvent: unknown }).TouchEvent = FakeTouchEvent;
});

function makeTarget(): EventTarget {
  const ElementCtor = (globalThis as unknown as { Element: new () => EventTarget }).Element;
  return new ElementCtor();
}

function makeReactTouchEvent(target: unknown, touches: FakeTouch[]) {
  return {
    target,
    touches,
    targetTouches: touches,
    changedTouches: touches,
    nativeEvent: { touches, targetTouches: touches, changedTouches: touches },
    preventDefault: vi.fn(),
  } as unknown as Parameters<typeof replayTouchAsMove>[0];
}

describe('replayTouchAsMove', () => {
  it('dispatches exactly one bubbling touchmove on the touch target, carrying the same touch points', () => {
    const target = makeTarget();
    const touch = new FakeTouch({ identifier: 0, clientX: 10, clientY: 20 });
    const received: FakeTouchEvent[] = [];
    target.addEventListener('touchmove', e => received.push(e as FakeTouchEvent));

    const result = replayTouchAsMove(makeReactTouchEvent(target, [touch]));

    expect(result).toBe(true);
    expect(received).toHaveLength(1);
    expect(received[0].bubbles).toBe(true);
    expect(received[0].touches).toEqual([touch]);
    expect(received[0].targetTouches).toEqual([touch]);
    expect(received[0].changedTouches).toEqual([touch]);
  });

  it('returns false and dispatches nothing when the TouchEvent constructor is missing', () => {
    const original = (globalThis as unknown as { TouchEvent: unknown }).TouchEvent;
    delete (globalThis as { TouchEvent?: unknown }).TouchEvent;
    try {
      const target = makeTarget();
      const received: FakeTouchEvent[] = [];
      target.addEventListener('touchmove', e => received.push(e as FakeTouchEvent));
      const touch = new FakeTouch({ identifier: 0, clientX: 0, clientY: 0 });

      const result = replayTouchAsMove(makeReactTouchEvent(target, [touch]));

      expect(result).toBe(false);
      expect(received).toHaveLength(0);
    } finally {
      (globalThis as unknown as { TouchEvent: unknown }).TouchEvent = original;
    }
  });

  it('returns false and dispatches nothing when there are no touches', () => {
    const target = makeTarget();
    const received: FakeTouchEvent[] = [];
    target.addEventListener('touchmove', e => received.push(e as FakeTouchEvent));

    const result = replayTouchAsMove(makeReactTouchEvent(target, []));

    expect(result).toBe(false);
    expect(received).toHaveLength(0);
  });

  it('returns false and dispatches nothing when the target is not an Element', () => {
    const touch = new FakeTouch({ identifier: 0, clientX: 0, clientY: 0 });

    const result = replayTouchAsMove(makeReactTouchEvent({}, [touch]));

    expect(result).toBe(false);
  });

  it('never calls preventDefault on the source event', () => {
    const target = makeTarget();
    const touch = new FakeTouch({ identifier: 0, clientX: 0, clientY: 0 });
    const event = makeReactTouchEvent(target, [touch]);

    replayTouchAsMove(event);

    expect(event.preventDefault).not.toHaveBeenCalled();
  });
});
