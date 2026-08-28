package geocode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/go-playground/validator/v10"
	"golang.org/x/sync/singleflight"
)

// ErrUnavailable is returned when the upstream failed and nothing is cached.
var ErrUnavailable = errors.New("geocode: upstream unavailable and no cache available")

// coordPrecision is the number of decimal places a coordinate is rounded to
// before it leaves this process. Two places is ~1.1 km — enough to name the
// settlement, coarse enough that a user's exact GPS fix is not handed to a
// third party, and it keeps the cache hit rate high.
const coordPrecision = 2

// lookupTimeout bounds a deduplicated upstream lookup.
const lookupTimeout = 15 * time.Second

// Service abstracts reverse-geocoding business logic.
type Service interface {
	Reverse(ctx context.Context, lat, lon float64) (*Place, error)
}

type service struct {
	repo     Repository
	client   Client
	validate *validator.Validate
	lookup   singleflight.Group
}

// NewService returns a cache-first reverse-geocoding Service.
func NewService(repo Repository, client Client) Service {
	return &service{
		repo:     repo,
		client:   client,
		validate: validator.New(),
	}
}

// NormaliseCoord rounds a coordinate to coordPrecision decimal places.
func NormaliseCoord(v float64) float64 {
	f := math.Pow(10, coordPrecision)
	return math.Round(v*f) / f
}

// Reverse resolves coordinates to a place name, serving from the local cache
// where possible so repeat lookups never reach the upstream.
func (s *service) Reverse(ctx context.Context, lat, lon float64) (*Place, error) {
	if err := s.validate.Struct(ReverseRequest{Lat: lat, Lon: lon}); err != nil {
		return nil, fmt.Errorf("geocode: invalid coords: %w", err)
	}

	nlat, nlon := NormaliseCoord(lat), NormaliseCoord(lon)

	if cached, err := s.repo.Get(ctx, nlat, nlon); err == nil {
		slog.InfoContext(ctx, "geocode: cache hit", "lat", nlat, "lon", nlon)
		place := cached.Place
		return &place, nil
	}

	key := fmt.Sprintf("%.2f,%.2f", nlat, nlon)
	v, err, _ := s.lookup.Do(key, func() (any, error) {
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lookupTimeout)
		defer cancel()

		place, err := s.client.Reverse(fetchCtx, nlat, nlon)
		if err != nil {
			slog.WarnContext(fetchCtx, "geocode: upstream lookup failed", "error", err, "lat", nlat, "lon", nlon)
			return nil, ErrUnavailable
		}

		if err := s.repo.Set(fetchCtx, &CachedPlace{Lat: nlat, Lon: nlon, Place: *place}); err != nil {
			slog.ErrorContext(fetchCtx, "geocode: failed to write cache", "error", err)
		}
		return place, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*Place), nil
}
