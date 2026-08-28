package geocode

import (
	"context"
	"errors"
	"time"
)

// ErrCacheMiss is returned by Repository.Get when no cached entry exists.
var ErrCacheMiss = errors.New("geocode: cache miss")

// Repository abstracts the reverse-geocode cache.
// Implementations must be safe for concurrent use.
type Repository interface {
	// Get retrieves a cached place for the given coordinates, or ErrCacheMiss.
	Get(ctx context.Context, lat, lon float64) (*CachedPlace, error)

	// Set inserts or updates the cached place for the given coordinates.
	Set(ctx context.Context, p *CachedPlace) error

	// DeleteStale removes entries older than retention and returns how many
	// rows were deleted.
	DeleteStale(ctx context.Context, retention time.Duration) (int64, error)
}

// Client abstracts the upstream reverse-geocoding interface.
// Implementations must be safe for concurrent use.
type Client interface {
	// Reverse resolves coordinates to a place description.
	Reverse(ctx context.Context, lat, lon float64) (*Place, error)
}
