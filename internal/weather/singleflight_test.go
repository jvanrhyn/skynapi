package weather_test

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jvanrhyn/skynapi/internal/weather"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingClient records how many upstream fetches actually happen and holds
// each one open long enough for concurrent callers to pile up behind it.
type countingClient struct {
	calls   atomic.Int64
	release chan struct{}
}

func (c *countingClient) Fetch(ctx context.Context, lat, lon float64, _ weather.FetchOptions) (*weather.FetchResult, error) {
	c.calls.Add(1)
	<-c.release
	exp := time.Now().Add(time.Hour)
	return &weather.FetchResult{
		Raw:       json.RawMessage(`{"type":"Feature"}`),
		ExpiresAt: &exp,
	}, nil
}

// missRepo always misses, so every caller is pushed onto the refresh path.
type missRepo struct {
	mu   sync.Mutex
	sets int
}

func (r *missRepo) Get(context.Context, float64, float64) (*weather.CachedWeather, error) {
	return nil, weather.ErrCacheMiss
}
func (r *missRepo) Set(context.Context, *weather.CachedWeather) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sets++
	return nil
}
func (r *missRepo) DeleteStale(context.Context, time.Duration) (int64, error) { return 0, nil }

// A burst of concurrent requests for one coordinate must collapse into a
// single upstream call, or an expiring cache entry turns into a burst of
// identical calls against the api.met.no quota.
func TestGetWeather_ConcurrentRequestsShareOneUpstreamFetch(t *testing.T) {
	const callers = 25

	client := &countingClient{release: make(chan struct{})}
	repo := &missRepo{}
	svc := weather.NewService(repo, client)

	var wg sync.WaitGroup
	results := make([]*weather.WeatherResult, callers)
	errs := make([]error, callers)

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.GetWeather(context.Background(), 52.3676, 4.9041)
		}(i)
	}

	// Give every goroutine time to reach the flight group, then let the one
	// in-flight fetch complete.
	assert.Eventually(t, func() bool { return client.calls.Load() >= 1 }, time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	close(client.release)
	wg.Wait()

	assert.Equal(t, int64(1), client.calls.Load(), "concurrent callers must share one upstream fetch")
	assert.Equal(t, 1, repo.sets, "the shared result should be written once")

	for i := range results {
		require.NoError(t, errs[i])
		assert.JSONEq(t, `{"type":"Feature"}`, string(results[i].Data), "every caller gets the shared body")
	}
}

// Different coordinates must not be deduplicated against each other.
func TestGetWeather_DistinctCoordinatesFetchIndependently(t *testing.T) {
	client := &countingClient{release: make(chan struct{})}
	close(client.release) // no blocking needed here
	svc := weather.NewService(&missRepo{}, client)

	_, err := svc.GetWeather(context.Background(), 52.3676, 4.9041)
	require.NoError(t, err)
	_, err = svc.GetWeather(context.Background(), -33.9249, 18.4241)
	require.NoError(t, err)

	assert.Equal(t, int64(2), client.calls.Load())
}

// A caller that gives up must not cancel the shared refresh for everyone else.
func TestGetWeather_CallerCancellationDoesNotFailTheFlight(t *testing.T) {
	client := &countingClient{release: make(chan struct{})}
	svc := weather.NewService(&missRepo{}, client)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := svc.GetWeather(ctx, 52.3676, 4.9041)
		done <- err
	}()

	assert.Eventually(t, func() bool { return client.calls.Load() == 1 }, time.Second, time.Millisecond)
	cancel()
	close(client.release)

	require.NoError(t, <-done, "the detached fetch should still complete")
}
