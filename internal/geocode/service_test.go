package geocode_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jvanrhyn/skynapi/internal/geocode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRepo struct {
	mu     sync.Mutex
	stored map[string]*geocode.CachedPlace
	sets   int
}

func newStubRepo() *stubRepo {
	return &stubRepo{stored: map[string]*geocode.CachedPlace{}}
}

func key(lat, lon float64) string {
	return string(rune(int(lat*100))) + ":" + string(rune(int(lon*100)))
}

func (r *stubRepo) Get(_ context.Context, lat, lon float64) (*geocode.CachedPlace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.stored[key(lat, lon)]; ok {
		return p, nil
	}
	return nil, geocode.ErrCacheMiss
}

func (r *stubRepo) Set(_ context.Context, p *geocode.CachedPlace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sets++
	r.stored[key(p.Lat, p.Lon)] = p
	return nil
}

func (r *stubRepo) DeleteStale(context.Context, time.Duration) (int64, error) { return 0, nil }

type stubClient struct {
	calls atomic.Int64
	err   error
	seen  chan [2]float64
}

func (c *stubClient) Reverse(_ context.Context, lat, lon float64) (*geocode.Place, error) {
	c.calls.Add(1)
	if c.seen != nil {
		c.seen <- [2]float64{lat, lon}
	}
	if c.err != nil {
		return nil, c.err
	}
	return &geocode.Place{Label: "Johannesburg, South Africa", City: "Johannesburg", Country: "South Africa", CountryCode: "ZA"}, nil
}

// The whole point of the proxy is that repeat lookups never reach the upstream.
func TestReverse_SecondLookupIsServedFromCache(t *testing.T) {
	repo, client := newStubRepo(), &stubClient{}
	svc := geocode.NewService(repo, client)

	first, err := svc.Reverse(context.Background(), -26.2041, 28.0473)
	require.NoError(t, err)
	assert.Equal(t, "Johannesburg, South Africa", first.Label)

	second, err := svc.Reverse(context.Background(), -26.2041, 28.0473)
	require.NoError(t, err)
	assert.Equal(t, first.Label, second.Label)

	assert.Equal(t, int64(1), client.calls.Load(), "the cached answer must not hit the upstream again")
	assert.Equal(t, 1, repo.sets)
}

// Coordinates are rounded before they leave the process, so a user's precise
// GPS fix is never handed to a third party — and nearby callers share an entry.
func TestReverse_CoordinatesAreRoundedBeforeLeavingTheProcess(t *testing.T) {
	repo, client := newStubRepo(), &stubClient{seen: make(chan [2]float64, 4)}
	svc := geocode.NewService(repo, client)

	_, err := svc.Reverse(context.Background(), -26.20413579, 28.04731234)
	require.NoError(t, err)

	sent := <-client.seen
	assert.Equal(t, -26.20, sent[0], "latitude must be rounded to 2 decimal places")
	assert.Equal(t, 28.05, sent[1], "longitude must be rounded to 2 decimal places")

	// A caller a few hundred metres away resolves to the same rounded key.
	_, err = svc.Reverse(context.Background(), -26.2039, 28.0468)
	require.NoError(t, err)
	assert.Equal(t, int64(1), client.calls.Load(), "a nearby lookup should reuse the cached entry")
}

func TestReverse_UpstreamFailureIsReportedAsUnavailable(t *testing.T) {
	svc := geocode.NewService(newStubRepo(), &stubClient{err: errors.New("nominatim down")})

	_, err := svc.Reverse(context.Background(), -26.2041, 28.0473)
	require.Error(t, err)
	assert.ErrorIs(t, err, geocode.ErrUnavailable)
}

func TestReverse_RejectsOutOfRangeCoordinates(t *testing.T) {
	client := &stubClient{}
	svc := geocode.NewService(newStubRepo(), client)

	_, err := svc.Reverse(context.Background(), 200, 0)
	require.Error(t, err)
	assert.Zero(t, client.calls.Load(), "invalid input must not reach the upstream")
}

func TestReverse_ConcurrentLookupsShareOneUpstreamCall(t *testing.T) {
	repo, client := newStubRepo(), &stubClient{}
	svc := geocode.NewService(repo, client)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.Reverse(context.Background(), -26.2041, 28.0473)
		}()
	}
	wg.Wait()

	assert.LessOrEqual(t, client.calls.Load(), int64(2),
		"a burst for one coordinate must not fan out to the upstream")
}

func TestNormaliseCoord(t *testing.T) {
	for _, tc := range []struct{ in, want float64 }{
		{-26.20413579, -26.20},
		{28.04731234, 28.05},
		{0, 0},
		{-0.005, -0.01},
	} {
		assert.Equal(t, tc.want, geocode.NormaliseCoord(tc.in))
	}
}
