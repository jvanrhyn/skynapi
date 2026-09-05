import test from 'node:test';
import assert from 'node:assert/strict';
import { bindDaySwipe } from '../caddy/html/swipe.mjs';

function setup() {
  const target = new EventTarget();
  target.setPointerCapture = () => {};
  const moves = [];
  bindDaySwipe(target, direction => moves.push(direction));
  const send = (type, x = 0, y = 0, extra = {}) => {
    const event = new Event(type, { cancelable: true });
    Object.assign(event, { pointerId: 1, isPrimary: true, button: 0, clientX: x, clientY: y, ...extra });
    target.dispatchEvent(event);
    return event;
  };
  return { moves, send };
}
test('horizontal swipes advance exactly one day in either direction', () => {
  const { moves, send } = setup();
  send('pointerdown', 150); send('pointerup', 50);
  send('pointerdown', 50); send('pointerup', 250);
  assert.deepEqual(moves, [1, -1]);
});
test('taps, short drags and diagonal gestures do not navigate', () => {
  const { moves, send } = setup();
  for (const [x,y] of [[0,0],[30,0],[50,40],[0,100]]) {
    send('pointerdown'); send('pointerup',x,y);
  }
  assert.deepEqual(moves, []);
});
test('vertical scrolling cannot become a swipe later in the gesture', () => {
  const { moves, send } = setup();
  send('pointerdown'); send('pointermove', 3, 30); send('pointerup', 100, 35);
  assert.deepEqual(moves, []);
});
test('cancelled, interrupted and multi-pointer gestures do not navigate', () => {
  for (const event of ['pointercancel', 'lostpointercapture', 'pointerdown']) {
    const { moves, send } = setup();
    send('pointerdown'); send(event, 0, 0, { isPrimary: false, pointerId: 2 }); send('pointerup', 100);
    assert.deepEqual(moves, []);
  }
});
test('keyboard arrows navigate while modified shortcuts remain untouched', () => {
  const { moves, send } = setup();
  assert.equal(send('keydown',0,0,{key:'ArrowRight'}).defaultPrevented,true);
  send('keydown',0,0,{key:'ArrowLeft'});
  assert.equal(send('keydown',0,0,{key:'ArrowLeft',altKey:true}).defaultPrevented,false);
  assert.deepEqual(moves,[1,-1]);
});
