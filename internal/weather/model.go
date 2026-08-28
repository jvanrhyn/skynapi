package weather

import (
	"encoding/json"
	"time"
)

// CachedWeather is the entity stored in the weather_cache table.
//
// Data holds the upstream response bytes verbatim. Nothing here re-serialises
// the forecast, so fields api.met.no adds later reach clients without a code
// change.
type CachedWeather struct {
	Lat          float64
	Lon          float64
	CachedAt     time.Time
	ExpiresAt    *time.Time
	LastModified *time.Time
	Data         json.RawMessage
}

// WeatherResult is returned by the service with response metadata.
type WeatherResult struct {
	Data      json.RawMessage
	CachedAt  *time.Time
	ExpiresAt *time.Time // upstream freshness deadline; drives Cache-Control
	Source    string     // "cache", "stale-cache" or "upstream"
}

// FetchOptions are optional headers to send with a MET request.
type FetchOptions struct {
	IfModifiedSince *time.Time
}

// FetchResult is returned by the MET client.
type FetchResult struct {
	// Raw is the upstream response body, verified to be syntactically valid
	// JSON. It is nil when the server returned 304 Not Modified.
	Raw          json.RawMessage
	NotModified  bool
	ExpiresAt    *time.Time
	LastModified *time.Time
}

// WeatherRequest carries validated coordinates for a weather lookup.
type WeatherRequest struct {
	Lat float64 `validate:"min=-90,max=90"`
	Lon float64 `validate:"min=-180,max=180"`
}
