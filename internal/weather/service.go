package weather

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-playground/validator/v10"
	"golang.org/x/sync/singleflight"
)

// Service abstracts weather business logic.
type Service interface {
	GetWeather(ctx context.Context, lat, lon float64) (*WeatherResult, error)
}

// ErrUpstreamUnavailable is returned when the upstream weather API failed
// and no cached data is available to fall back to.
var ErrUpstreamUnavailable = errors.New("weather: upstream unavailable and no cache available")

// refreshTimeout bounds a deduplicated upstream refresh. It sits above the
// HTTP client's own 10s timeout so the client's error surfaces first.
const refreshTimeout = 15 * time.Second

type service struct {
	repo     Repository
	client   Client
	validate *validator.Validate
	// refresh deduplicates concurrent upstream fetches for the same
	// coordinate so a cache expiry cannot fan out into a burst of identical
	// calls against the api.met.no quota.
	refresh singleflight.Group
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
//  3. Otherwise          → one deduplicated refresh per coordinate:
//     conditional fetch, update cache on 200, bump TTL on 304,
//     fall back to stale cache on upstream error, 503 if there is none.
func (s *service) GetWeather(ctx context.Context, lat, lon float64) (*WeatherResult, error) {
	req := WeatherRequest{Lat: lat, Lon: lon}
	if err := s.validate.Struct(req); err != nil {
		return nil, fmt.Errorf("weather: invalid coords: %w", err)
	}

	nlat, nlon := NormaliseCoord(lat), NormaliseCoord(lon)

	cached, cacheErr := s.repo.Get(ctx, nlat, nlon)

	// Cache hit + fresh — serve without touching upstream or the flight group.
	if cacheErr == nil && cached.ExpiresAt != nil && time.Now().Before(*cached.ExpiresAt) {
		slog.InfoContext(ctx, "weather: returning fresh cache", "lat", nlat, "lon", nlon)
		return &WeatherResult{Data: cached.Data, CachedAt: &cached.CachedAt, ExpiresAt: cached.ExpiresAt, Source: "cache"}, nil
	}

	key := fmt.Sprintf("%.4f,%.4f", nlat, nlon)
	v, err, _ := s.refresh.Do(key, func() (any, error) {
		// Detach cancellation so an abandoned request cannot fail the refresh
		// for everyone sharing this flight; request-scoped log values survive.
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refreshTimeout)
		defer cancel()
		return s.fetchAndStore(fetchCtx, nlat, nlon, cached, cacheErr)
	})
	if err != nil {
		return nil, err
	}
	return v.(*WeatherResult), nil
}

// fetchAndStore performs the upstream call and cache write for one coordinate.
// cached/cacheErr are the caller's cache lookup: cacheErr == nil means cached
// holds a usable (but expired) entry to revalidate against or fall back to.
func (s *service) fetchAndStore(ctx context.Context, nlat, nlon float64, cached *CachedWeather, cacheErr error) (*WeatherResult, error) {
	// Conditional GET when we have a stale entry to revalidate.
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
			return &WeatherResult{Data: cached.Data, CachedAt: &cached.CachedAt, ExpiresAt: cached.ExpiresAt, Source: "stale-cache"}, nil
		}
		return nil, ErrUpstreamUnavailable
	}

	if result.NotModified {
		// Upstream confirms the cached body is still current; extend its TTL.
		if cacheErr == nil {
			updated := *cached
			updated.ExpiresAt = result.ExpiresAt
			if err := s.repo.Set(ctx, &updated); err != nil {
				slog.ErrorContext(ctx, "weather: failed to update cache TTL", "error", err)
			}
			slog.InfoContext(ctx, "weather: returning revalidated cache (304 not modified)", "lat", nlat, "lon", nlon)
			return &WeatherResult{Data: cached.Data, CachedAt: &cached.CachedAt, ExpiresAt: result.ExpiresAt, Source: "cache"}, nil
		}
		// 304 without a cache entry to back it should never happen (we only send
		// If-Modified-Since when we have one), but guard against caching a nil body.
		return nil, fmt.Errorf("weather: upstream returned 304 with no cached entry")
	}

	entry := &CachedWeather{
		Lat:          nlat,
		Lon:          nlon,
		ExpiresAt:    result.ExpiresAt,
		LastModified: result.LastModified,
		Data:         result.Raw,
	}
	if err := s.repo.Set(ctx, entry); err != nil {
		slog.ErrorContext(ctx, "weather: failed to write cache", "error", err)
	}

	now := time.Now()
	return &WeatherResult{Data: result.Raw, CachedAt: &now, ExpiresAt: result.ExpiresAt, Source: "upstream"}, nil
}
