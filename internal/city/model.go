package city

// City is the domain model returned by the search endpoint.
// Coordinates come from the all_countries Geonames dataset.
type City struct {
	GeonameID   int64   `json:"id"`
	Name        string  `json:"name"`
	CountryCode string  `json:"country"`
	CountryName string  `json:"country_name"`
	Region      string  `json:"region"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Timezone    string  `json:"timezone"`
}

// SearchParams holds validated query parameters for city search.
//
// Q has a two-character floor: a single character matches a large share of the
// Geonames table for no useful result, and the trigram index cannot narrow it.
type SearchParams struct {
	Q     string `validate:"required,min=2,max=100"`
	Page  int    `validate:"min=1"`
	Limit int    `validate:"min=1,max=100"`
}

// SearchResult is the paginated response from the search endpoint.
type SearchResult struct {
	Cities []City `json:"cities"`
	// Total saturates at maxCountedMatches rather than counting every match.
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}
