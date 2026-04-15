package weather

import (
	"context"
)

// Client abstracts the upstream api.met.no HTTP interface.
// Implementations must be safe for concurrent use.
type Client interface {
	// Fetch retrieves a weather forecast for the given coordinates.
	// opts.IfModifiedSince triggers a conditional GET; FetchResult.NotModified
	// will be true and FetchResult.Response will be nil on a 304 response.
	Fetch(ctx context.Context, lat, lon float64, opts FetchOptions) (*FetchResult, error)
}
