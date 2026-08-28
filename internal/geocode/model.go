package geocode

import "time"

// Place is the resolved description of a coordinate.
type Place struct {
	Label       string `json:"label"`
	City        string `json:"city"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
}

// CachedPlace is the entity stored in the reverse_geocode_cache table.
type CachedPlace struct {
	Lat      float64
	Lon      float64
	CachedAt time.Time
	Place    Place
}

// ReverseRequest carries validated coordinates for a reverse lookup.
type ReverseRequest struct {
	Lat float64 `validate:"min=-90,max=90"`
	Lon float64 `validate:"min=-180,max=180"`
}
