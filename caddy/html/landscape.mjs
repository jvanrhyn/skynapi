// Presentation state derived from an already processed day; no DOM or clock.
export function landscapeState(day) {
  const symbol = (day.sym || '').replace(/_(day|night|polartwilight)$/, '');
  const snow = symbol.includes('snow');
  const sleet = symbol.includes('sleet');
  const storm = symbol.includes('thunder');
  const fog = symbol.includes('fog');
  const rain = sleet || symbol.includes('rain') || (!snow && !fog && day.rain > 0);
  const clouds = rain || snow || storm || symbol === 'cloudy' ? 'overcast'
    : symbol === 'fair' || symbol === 'partlycloudy' ? 'broken'
    : symbol === 'clearsky' ? 'clear' : 'overcast';
  return { clouds, rain, snow: snow || sleet, storm, fog,
    // A visual cue at 25 km/h, not a weather warning threshold.
    windy: Number.isFinite(day.maxW) && day.maxW >= 25,
    sun: !fog && !storm && (clouds !== 'overcast' || symbol.includes('showers')) };
}
