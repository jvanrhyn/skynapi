import test from 'node:test';
import assert from 'node:assert/strict';
import { landscapeState } from '../caddy/html/landscape.mjs';

const scene = (sym, extra = {}) => landscapeState({ sym, rain: 0, maxW: 10, ...extra });
test('clear, broken, and overcast skies follow selected day symbols', () => {
  assert.equal(scene('clearsky').sun, true);
  assert.equal(scene('clearsky').clouds, 'clear');
  assert.equal(scene('partlycloudy').clouds, 'broken');
  assert.equal(scene('cloudy').sun, false);
});
test('showers retain sun, steady rain and storms obscure it', () => {
  assert.equal(scene('lightrainshowers').sun, true);
  assert.equal(scene('rain').rain, true);
  assert.equal(scene('rain').sun, false);
  assert.equal(scene('rainandthunder').storm, true);
  assert.equal(scene('rainandthunder').sun, false);
});
test('snow and sleet are not drawn as ordinary rain', () => {
  assert.equal(scene('snow', { rain: 4 }).rain, false);
  assert.equal(scene('snow').snow, true);
  assert.equal(scene('sleet').snow, true);
  assert.equal(scene('sleet').rain, true);
});
test('fog hides sun and adds mist', () => {
  assert.equal(scene('fog').fog, true);
  assert.equal(scene('fog').sun, false);
});
test('daily precipitation adds rain even if representative noon symbol is clear', () => {
  assert.equal(scene('clearsky', { rain: 2 }).rain, true);
});
test('wind cue follows maximum wind, and resetting day clears every effect', () => {
  assert.equal(scene('cloudy', { maxW: 25 }).windy, true);
  assert.equal(scene('cloudy', { maxW: 24 }).windy, false);
  const clear = scene('clearsky');
  for (const key of ['rain', 'snow', 'storm', 'fog', 'windy']) assert.equal(clear[key], false);
});
test('unknown symbols have a neutral cloudy fallback; day/night suffixes are supported', () => {
  assert.equal(scene('unknown').clouds, 'overcast');
  assert.equal(scene('clearsky_day').sun, true);
});
