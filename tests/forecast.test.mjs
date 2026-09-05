import test from 'node:test';
import assert from 'node:assert/strict';
import { processData } from '../caddy/html/forecast.mjs';

// Small MET-shaped fixtures keep the scenarios readable and require no network.
function entry(time, { temp = 20, wind = 5, gust, pressure = 1000, h1, h6,
  symbol = 'clearsky_day' } = {}) {
  const data = { instant: { details: {
    air_temperature: temp, wind_speed: wind, wind_speed_of_gust: gust,
    air_pressure_at_sea_level: pressure, wind_from_direction: 90,
  } } };
  for (const [hours, rain] of [[1, h1], [6, h6]]) {
    if (rain !== undefined) data[`next_${hours}_hours`] = {
      details: { precipitation_amount: rain }, summary: { symbol_code: symbol },
    };
  }
  return { time, data };
}

function forecast(entries, zone = 'UTC', now = '2026-09-05T12:00:00Z') {
  return processData({ geometry: { coordinates: [28.0473, -26.2041] },
    properties: { timeseries: entries } }, zone, new Date(now));
}

test('daily summary uses unrounded measurements and converts wind to km/h', () => {
  const { days, locStr } = forecast([
    entry('2026-09-05T11:00:00Z', { temp: 12.4, wind: 2, pressure: 1000, h1: 0.1 }),
    entry('2026-09-05T12:00:00Z', { temp: 25.6, wind: 5.5, gust: 10, pressure: 1003, h1: 0.2 }),
  ]);
  assert.equal(days.length, 1);
  const day = days[0];
  assert.deepEqual([day.minT, day.maxT, day.maxW, day.maxG, day.avgP, day.rain],
    [12, 26, 20, 36, 1001.5, 0.3]);
  assert.equal(day.label, 'Sunny');
  assert.equal(day.entries[1].wind, 20);
  assert.equal(day.entries[1].dir, 90);
  assert.equal(locStr, '26.2041°S,\u00a028.0473°E');
});

test('groups by the selected city day and determines today using that city clock', () => {
  const { days, todayIdx } = forecast([
    entry('2026-09-05T20:00:00Z'), entry('2026-09-05T21:00:00Z'),
    entry('2026-09-05T22:00:00Z'), entry('2026-09-05T23:00:00Z'),
  ], 'Africa/Johannesburg', '2026-09-05T22:30:00Z');
  assert.deepEqual(days.map(d => d.key), ['2026-09-05', '2026-09-06']);
  assert.deepEqual(days.map(d => d.entries.map(e => e.lh)), [[22, 23], [0, 1]]);
  assert.equal(todayIdx, 1);
});

test('handles fractional timezone offsets', () => {
  const { days } = forecast([
    entry('2026-09-05T18:00:00Z'), entry('2026-09-05T18:10:00Z'),
    entry('2026-09-05T19:00:00Z'), entry('2026-09-05T20:00:00Z'),
  ], 'Asia/Kathmandu');
  assert.deepEqual(days.map(d => d.key), ['2026-09-05', '2026-09-06']);
  assert.deepEqual(days[1].entries.map(e => e.lh), [0, 1]);
});

test('keeps both repeated hours during daylight saving fall-back', () => {
  const { days } = forecast([
    entry('2026-11-01T05:00:00Z', { h1: 1 }),
    entry('2026-11-01T06:00:00Z', { h1: 2 }),
    entry('2026-11-01T07:00:00Z', { h1: 3 }),
  ], 'America/New_York');
  assert.deepEqual(days[0].entries.map(e => e.lh), [1, 1, 2]);
  assert.equal(days[0].rain, 6);
});

test('skips the missing hour during daylight saving spring-forward', () => {
  const { days } = forecast([
    entry('2026-03-08T06:00:00Z'), entry('2026-03-08T07:00:00Z'),
  ], 'America/New_York');
  assert.deepEqual(days[0].entries.map(e => e.lh), [1, 3]);
});

test('counts hourly then six-hour rain without overlapping six-hour windows', () => {
  const { days } = forecast([
    entry('2026-09-05T00:00:00Z', { h1: 1, h6: 99 }),
    entry('2026-09-05T01:00:00Z', { h1: 2, h6: 99 }),
    entry('2026-09-05T02:00:00Z', { h6: 6 }),
    entry('2026-09-05T03:00:00Z', { h6: 99 }),
    entry('2026-09-05T08:00:00Z', { h6: 12 }),
  ], 'Africa/Johannesburg');
  assert.equal(days[0].rain, 21);
  assert.deepEqual(days[0].entries.map(e => e.rainHours), [1, 1, 6, 0, 6]);
});

test('preserves interval-start attribution for six-hour rain crossing midnight', () => {
  // MET gives a total for the interval, not a measured per-hour distribution.
  // Existing behavior assigns the entire block to its starting local day.
  const { days } = forecast([
    entry('2026-09-05T18:00:00Z', { h6: 6 }),
    entry('2026-09-05T21:00:00Z', { h6: 99 }),
    entry('2026-09-06T00:00:00Z', { h6: 12 }),
    entry('2026-09-06T06:00:00Z', { h6: 3 }),
  ], 'Africa/Johannesburg');
  assert.deepEqual(days.map(d => d.rain), [6, 15]);
});

test('partial days use available samples; days with fewer than two are omitted', () => {
  const { days, todayIdx } = forecast([
    entry('2026-09-05T23:00:00Z'),
    entry('2026-09-06T22:00:00Z', { temp: 10 }),
    entry('2026-09-06T23:00:00Z', { temp: 12 }),
    entry('2026-09-07T00:00:00Z'),
  ]);
  assert.deepEqual(days.map(d => d.key), ['2026-09-06']);
  assert.deepEqual([days[0].minT, days[0].maxT, todayIdx], [10, 12, 0]);
});

test('missing rain and gusts retain their entry-level unknown values', () => {
  const { days } = forecast([
    entry('2026-09-05T11:00:00Z'), entry('2026-09-05T12:00:00Z'),
  ]);
  assert.equal(days[0].maxG, null);
  assert.equal(days[0].entries[0].rain, null);
  assert.equal(days[0].entries[0].gust, null);
  assert.equal(days[0].label, 'Cloudy');
});

test('invalid timezone falls back to the runtime timezone', () => {
  const entries = [entry('2026-09-05T11:00:00Z'), entry('2026-09-05T12:00:00Z')];
  assert.deepEqual(forecast(entries, 'invalid/zone'), forecast(entries, null));
});

test('empty timeseries is safe for the calculation module', () => {
  const result = forecast([]);
  assert.deepEqual(result.days, []);
  assert.equal(result.todayIdx, 0);
});

test('processing does not mutate the source forecast', () => {
  const entries = [entry('2026-09-05T11:00:00Z'), entry('2026-09-05T12:00:00Z')];
  const original = structuredClone(entries);
  forecast(entries);
  assert.deepEqual(entries, original);
});
