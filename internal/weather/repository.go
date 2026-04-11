package weather

import (
	"context"
	"errors"
)

// ErrCacheMiss is returned by Repository.Get when no cached entry exists.
var ErrCacheMiss = errors.New("weather: cache miss")

// Repository abstracts the weather cache persistence layer.
// Implementations must be safe for concurrent use.
type Repository interface {
	// Get retrieves a cached entry for the given coordinates.
	// Coordinates are normalised to 4 decimal places before lookup.
	// Returns ErrCacheMiss if no entry is found.
	Get(ctx context.Context, lat, lon float64) (*CachedWeather, error)

	// Set inserts or updates the cached weather for the given coordinates.
	Set(ctx context.Context, w *CachedWeather) error
}
