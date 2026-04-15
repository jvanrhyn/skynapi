package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-playground/validator/v10"
)

// Service abstracts weather business logic.
type Service interface {
	GetWeather(ctx context.Context, lat, lon float64) (json.RawMessage, error)
}

// ErrUpstreamUnavailable is returned when the upstream weather API failed
// and no cached data is available to fall back to.
var ErrUpstreamUnavailable = errors.New("weather: upstream unavailable and no cache available")

type service struct {
	repo     Repository
	client   Client
	validate *validator.Validate
}

// NewService returns a Service implementing a cache-first strategy.
func NewService(repo Repository, client Client) Service {
	return &service{
		repo:     repo,
		client:   client,
		validate: validator.New(),
	}
}

// GetWeather returns weather JSON for the given coordinates.
//
// Strategy:
//  1. Normalise coordinates.
//  2. Cache hit + fresh  → return cached data immediately.
//  3. Cache hit + stale  → conditional fetch; update cache on 200, bump TTL on 304,
//     fall back to stale on upstream error.
//  4. Cache miss         → full fetch; store on 200, return 503 on error.
func (s *service) GetWeather(ctx context.Context, lat, lon float64) (json.RawMessage, error) {
	req := WeatherRequest{Lat: lat, Lon: lon}
	if err := s.validate.Struct(req); err != nil {
		return nil, fmt.Errorf("weather: invalid coords: %w", err)
	}

	nlat, nlon := NormaliseCoord(lat), NormaliseCoord(lon)

	cached, cacheErr := s.repo.Get(ctx, nlat, nlon)

	// Cache hit + fresh.
	if cacheErr == nil && cached.ExpiresAt != nil && time.Now().Before(*cached.ExpiresAt) {
		return cached.Data, nil
	}

	// Build fetch options (conditional GET when we have a stale entry).
	var opts FetchOptions
	if cacheErr == nil && cached.LastModified != nil {
		opts.IfModifiedSince = cached.LastModified
	}

	result, fetchErr := s.client.Fetch(ctx, nlat, nlon, opts)

	if fetchErr != nil {
		// Upstream failed — return stale cache if available.
		if cacheErr == nil {
			slog.WarnContext(ctx, "weather: upstream error, returning stale cache",
				"error", fetchErr, "lat", nlat, "lon", nlon)
			return cached.Data, nil
		}
		return nil, ErrUpstreamUnavailable
	}

	if result.NotModified {
		// Upstream confirms cache is still valid — bump TTL in background.
		if cacheErr == nil {
			updated := *cached
			updated.ExpiresAt = result.ExpiresAt
			if err := s.repo.Set(ctx, &updated); err != nil {
				slog.ErrorContext(ctx, "weather: failed to update cache TTL", "error", err)
			}
			return cached.Data, nil
		}
	}

	// Fresh upstream data — serialise and cache.
	raw, err := json.Marshal(result.Response)
	if err != nil {
		return nil, fmt.Errorf("weather: marshal response: %w", err)
	}

	entry := &CachedWeather{
		Lat:          nlat,
		Lon:          nlon,
		ExpiresAt:    result.ExpiresAt,
		LastModified: result.LastModified,
		Data:         raw,
	}
	if err := s.repo.Set(ctx, entry); err != nil {
		slog.ErrorContext(ctx, "weather: failed to write cache", "error", err)
	}

	return raw, nil
}
