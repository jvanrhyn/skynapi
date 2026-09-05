// ── TIME ZONES ────────────────────────────────────────────────────
// Forecasts are rendered in the *city's* local time, not the viewer's. The
// city search returns an IANA zone; "current location" has none and correctly
// falls back to the browser's own zone.
function resolveZone(timezone) {
  if (!timezone) return null;
  try {
    new Intl.DateTimeFormat('en', { timeZone: timezone });
    return timezone;
  } catch (_) {
    return null; // unknown zone name — fall back to the browser
  }
}

// zonedPartsFormatter returns a formatter that yields calendar fields in zone,
// or in the browser's zone when zone is null.
function zonedPartsFormatter(zone) {
  const opts = { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', hourCycle: 'h23' };
  if (zone) opts.timeZone = zone;
  return new Intl.DateTimeFormat('en-CA', opts);
}

// zonedParts maps an instant to its calendar day key and hour in the
// formatter's zone. Reading named parts keeps this independent of how the
// locale happens to order them.
function zonedParts(date, fmt) {
  const p = {};
  for (const part of fmt.formatToParts(date)) p[part.type] = part.value;
  return { key: `${p.year}-${p.month}-${p.day}`, hour: Number(p.hour) % 24 };
}

// ── DATA PROCESSING ──────────────────────────────────────────────
export function processData(json, timezone, now = new Date()) {
  const ts = json.properties.timeseries;
  const [lon, lat] = json.geometry.coordinates;
  const locStr = Math.abs(lat).toFixed(4) + '°' + (lat < 0 ? 'S' : 'N') + ',\xa0' + Math.abs(lon).toFixed(4) + '°' + (lon < 0 ? 'W' : 'E');

  const zone = resolveZone(timezone);
  const fmt  = zonedPartsFormatter(zone);

  // Precipitation arrives as hourly blocks for roughly the first two days and
  // then as 6-hour blocks aligned to UTC 00/06/12/18. Selecting the 6-hour
  // slots by local hour cannot work: for any zone whose offset is not a
  // multiple of six hours they never line up, and the long-range days silently
  // total zero. Walk the series in order instead, tracking how far ahead
  // precipitation has already been counted, so each hour is counted exactly
  // once whatever the zone.
  const byDay = new Map();
  let coveredUntil = 0;

  for (const e of ts) {
    const at = new Date(e.time);
    const { key, hour } = zonedParts(at, fmt);
    if (!byDay.has(key)) byDay.set(key, []);

    const t  = at.getTime();
    const d  = e.data.instant.details;
    const h1 = e.data.next_1_hours?.details?.precipitation_amount;
    const h6 = e.data.next_6_hours?.details?.precipitation_amount;

    let rain = null, rainHours = 0;
    if (h1 != null) {
      rain = h1; rainHours = 1;
      coveredUntil = t + 3600000;
    } else if (h6 != null && t >= coveredUntil) {
      rain = h6; rainHours = 6;
      coveredUntil = t + 6 * 3600000;
    }

    const sym1h = (e.data.next_1_hours?.summary?.symbol_code ||
                   e.data.next_6_hours?.summary?.symbol_code || 'cloudy')
                  .replace(/_(day|night|polartwilight)$/, '');

    byDay.get(key).push({
      raw:      e,
      lh:       hour,
      temp:     Math.round(d.air_temperature),
      wind:     Math.round(d.wind_speed * 3.6),
      gust:     d.wind_speed_of_gust != null ? Math.round(d.wind_speed_of_gust * 3.6) : null,
      dir:      Math.round(d.wind_from_direction ?? 0),
      pressure: Math.round(d.air_pressure_at_sea_level),
      rain,
      rainHours,
      sym:      sym1h,
    });
  }

  const days = [];
  for (const [key, entries] of byDay) {
    if (entries.length < 2) continue;
    const raws      = entries.map(e => e.raw);
    const temps     = raws.map(e => e.data.instant.details.air_temperature);
    const winds     = raws.map(e => e.data.instant.details.wind_speed);
    const gustRaw   = raws.map(e => e.data.instant.details.wind_speed_of_gust).filter(v => v != null);
    const pressures = raws.map(e => e.data.instant.details.air_pressure_at_sea_level);
    const rain      = entries.reduce((sum, e) => sum + (e.rain ?? 0), 0);

    const noon = entries.reduce((b, e) => Math.abs(e.lh - 12) < Math.abs(b.lh - 12) ? e : b);
    const sym = noon.raw.data.next_6_hours?.summary?.symbol_code ||
                noon.raw.data.next_1_hours?.summary?.symbol_code ||
                noon.raw.data.next_12_hours?.summary?.symbol_code || 'cloudy';
    const base = sym.replace(/_(day|night|polartwilight)$/, '');

    days.push({
      key,
      // Local midnight of the key. The key already names the day in the city's
      // zone, so formatting this without a timeZone reproduces that same date.
      date: new Date(key + 'T00:00:00'),
      maxT: Math.round(Math.max(...temps)),
      minT: Math.round(Math.min(...temps)),
      rain: Math.round(rain * 10) / 10,
      maxW: Math.round(Math.max(...winds) * 3.6),
      maxG: gustRaw.length ? Math.round(Math.max(...gustRaw) * 3.6) : null,
      avgP: Math.round(pressures.reduce((a, b) => a + b, 0) / pressures.length * 10) / 10,
      sym: base,
      label: LABELS[base] || 'Mixed',
      entries,
    });
  }

  // "Today" is today in the city being viewed, not where the viewer is sitting.
  const nowKey = zonedParts(now, fmt).key;
  const todayIdx = Math.max(0, days.findIndex(d => d.key === nowKey));

  return { days, locStr, todayIdx, zone };
}

const LABELS = {
  clearsky:'Sunny', fair:'Mostly clear', partlycloudy:'Partly cloudy',
  cloudy:'Cloudy', fog:'Fog', lightfog:'Foggy',
  lightrainshowers:'Light showers', rainshowers:'Showers', heavyrainshowers:'Heavy showers',
  lightrain:'Light rain', rain:'Rainy', heavyrain:'Heavy rain',
  lightsleet:'Light sleet', sleet:'Sleet', heavysleet:'Heavy sleet',
  lightsleetshowers:'Sleet showers', sleetshowers:'Sleet showers', heavysleetshowers:'Heavy sleet',
  lightsnowshowers:'Snow showers', snowshowers:'Snow showers', heavysnowshowers:'Heavy snow',
  lightsnow:'Light snow', snow:'Snow', heavysnow:'Heavy snow',
  thunder:'Thunder', lightrainandthunder:'Stormy', rainandthunder:'Stormy',
  heavyrainandthunder:'Severe storm', lightssleetandthunder:'Sleet storm',
};

