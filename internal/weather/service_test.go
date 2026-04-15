package weather_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jvanrhyn/skynapi/internal/weather"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Mock Repository ---

type mockRepo struct{ mock.Mock }

func (m *mockRepo) Get(ctx context.Context, lat, lon float64) (*weather.CachedWeather, error) {
	args := m.Called(ctx, lat, lon)
	r, _ := args.Get(0).(*weather.CachedWeather)
	return r, args.Error(1)
}

func (m *mockRepo) Set(ctx context.Context, w *weather.CachedWeather) error {
	return m.Called(ctx, w).Error(0)
}

// --- Mock Client ---

type mockClient struct{ mock.Mock }

func (m *mockClient) Fetch(ctx context.Context, lat, lon float64, opts weather.FetchOptions) (*weather.FetchResult, error) {
	args := m.Called(ctx, lat, lon, opts)
	r, _ := args.Get(0).(*weather.FetchResult)
	return r, args.Error(1)
}

// --- Helpers ---

func freshCached(lat, lon float64) *weather.CachedWeather {
	exp := time.Now().Add(1 * time.Hour)
	lm := time.Now().Add(-30 * time.Minute)
	return &weather.CachedWeather{
		Lat: lat, Lon: lon,
		ExpiresAt:    &exp,
		LastModified: &lm,
		Data:         json.RawMessage(`{"type":"Feature"}`),
	}
}

func staleCached(lat, lon float64) *weather.CachedWeather {
	exp := time.Now().Add(-1 * time.Hour)
	lm := time.Now().Add(-2 * time.Hour)
	return &weather.CachedWeather{
		Lat: lat, Lon: lon,
		ExpiresAt:    &exp,
		LastModified: &lm,
		Data:         json.RawMessage(`{"type":"Feature","stale":true}`),
	}
}

func TestService_GetWeather(t *testing.T) {
	const lat, lon = 52.3676, 4.9041

	freshResult := &weather.FetchResult{
		Response:  &weather.METResponse{Type: "Feature"},
		ExpiresAt: func() *time.Time { t := time.Now().Add(1 * time.Hour); return &t }(),
	}

	tests := []struct {
		name        string
		lat, lon    float64
		setupRepo   func(*mockRepo)
		setupClient func(*mockClient)
		wantErr     bool
		wantErrIs   error
		checkResult func(t *testing.T, data json.RawMessage)
	}{
		{
			name: "cache hit + fresh — no upstream call",
			lat:  lat, lon: lon,
			setupRepo: func(r *mockRepo) {
				r.On("Get", mock.Anything, weather.NormaliseCoord(lat), weather.NormaliseCoord(lon)).
					Return(freshCached(lat, lon), nil)
			},
			setupClient: func(c *mockClient) { /* no calls expected */ },
			checkResult: func(t *testing.T, data json.RawMessage) {
				assert.Contains(t, string(data), "Feature")
			},
		},
		{
			name: "cache miss — fetch and store",
			lat:  lat, lon: lon,
			setupRepo: func(r *mockRepo) {
				r.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(nil, weather.ErrCacheMiss)
				r.On("Set", mock.Anything, mock.Anything).Return(nil)
			},
			setupClient: func(c *mockClient) {
				c.On("Fetch", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(freshResult, nil)
			},
			checkResult: func(t *testing.T, data json.RawMessage) {
				assert.NotEmpty(t, data)
			},
		},
		{
			name: "stale cache + upstream 304 — return stale, bump TTL",
			lat:  lat, lon: lon,
			setupRepo: func(r *mockRepo) {
				r.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(staleCached(lat, lon), nil)
				r.On("Set", mock.Anything, mock.Anything).Return(nil)
			},
			setupClient: func(c *mockClient) {
				c.On("Fetch", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(&weather.FetchResult{NotModified: true, ExpiresAt: freshResult.ExpiresAt}, nil)
			},
			checkResult: func(t *testing.T, data json.RawMessage) {
				assert.Contains(t, string(data), "stale")
			},
		},
		{
			name: "stale cache + upstream error — return stale gracefully",
			lat:  lat, lon: lon,
			setupRepo: func(r *mockRepo) {
				r.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(staleCached(lat, lon), nil)
			},
			setupClient: func(c *mockClient) {
				c.On("Fetch", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, errors.New("connection timeout"))
			},
			checkResult: func(t *testing.T, data json.RawMessage) {
				assert.Contains(t, string(data), "stale")
			},
		},
		{
			name: "cache miss + upstream error — ErrUpstreamUnavailable",
			lat:  lat, lon: lon,
			setupRepo: func(r *mockRepo) {
				r.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(nil, weather.ErrCacheMiss)
			},
			setupClient: func(c *mockClient) {
				c.On("Fetch", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, errors.New("connection refused"))
			},
			wantErr:   true,
			wantErrIs: weather.ErrUpstreamUnavailable,
		},
		{
			name:      "invalid coordinates — validation error",
			lat:       200, lon: 0,
			setupRepo: func(r *mockRepo) {},
			setupClient: func(c *mockClient) {},
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepo{}
			client := &mockClient{}
			tc.setupRepo(repo)
			tc.setupClient(client)

			svc := weather.NewService(repo, client)
			data, err := svc.GetWeather(context.Background(), tc.lat, tc.lon)

			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrIs != nil {
					assert.True(t, errors.Is(err, tc.wantErrIs))
				}
				return
			}
			require.NoError(t, err)
			if tc.checkResult != nil {
				tc.checkResult(t, data)
			}
			repo.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

func TestNormaliseCoord(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{52.36760001, 52.3676},
		{4.90412345, 4.9041},
		{-33.92491, -33.9249},
		{0.0, 0.0},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, weather.NormaliseCoord(tc.input))
	}
}
