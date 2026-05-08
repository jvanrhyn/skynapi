package weather

import (
	"encoding/json"
	"time"
)

// METResponse is the top-level response from api.met.no locationforecast/2.0/compact.
type METResponse struct {
	Type       string        `json:"type"`
	Geometry   METGeometry   `json:"geometry"`
	Properties METProperties `json:"properties"`
}

// METGeometry holds the coordinate point.
type METGeometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"` // [lon, lat, altitude]
}

// METProperties contains forecast metadata and the timeseries array.
type METProperties struct {
	Meta       METMeta         `json:"meta"`
	Timeseries []METTimeSeries `json:"timeseries"`
}

// METMeta contains units and update timestamp.
type METMeta struct {
	UpdatedAt string            `json:"updated_at"`
	Units     map[string]string `json:"units"`
}

// METTimeSeries is a single forecast step.
type METTimeSeries struct {
	Time string  `json:"time"`
	Data METData `json:"data"`
}

// METData holds instant measurements and short-range forecasts.
type METData struct {
	Instant     *METInstant  `json:"instant"`
	Next1Hours  *METForecast `json:"next_1_hours"`
	Next6Hours  *METForecast `json:"next_6_hours"`
	Next12Hours *METForecast `json:"next_12_hours"`
}

// METInstant is instantaneous sensor data.
type METInstant struct {
	Details map[string]float64 `json:"details"`
}

// METForecast is a short-range forecast block.
type METForecast struct {
	Summary METSummary         `json:"summary"`
	Details map[string]float64 `json:"details"`
}

// METSummary holds the symbol code for the forecast period.
type METSummary struct {
	SymbolCode string `json:"symbol_code"`
}

// CachedWeather is the entity stored in the weather_cache table.
type CachedWeather struct {
	Lat          float64
	Lon          float64
	CachedAt     time.Time
	ExpiresAt    *time.Time
	LastModified *time.Time
	Data         json.RawMessage // raw MET JSON blob
}

// WeatherResult is returned by the service with response metadata.
type WeatherResult struct {
	Data     json.RawMessage
	CachedAt *time.Time
	Source   string
}

// FetchOptions are optional headers to send with a MET request.
type FetchOptions struct {
	IfModifiedSince *time.Time
}

// FetchResult is returned by the MET client.
type FetchResult struct {
	// Response is nil when the server returned 304 Not Modified.
	Response     *METResponse
	NotModified  bool
	ExpiresAt    *time.Time
	LastModified *time.Time
}

// WeatherRequest carries validated coordinates for a weather lookup.
type WeatherRequest struct {
	Lat float64 `validate:"min=-90,max=90"`
	Lon float64 `validate:"min=-180,max=180"`
}
