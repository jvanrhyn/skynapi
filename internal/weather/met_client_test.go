package weather_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jvanrhyn/skynapi/internal/weather"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMETClient_ServesUpstreamBytesVerbatim(t *testing.T) {
	// A field the Go models never knew about must survive the round trip.
	const body = `{"type":"Feature","properties":{"meta":{"radar_coverage":"ok"}},"future_field":42}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "skynapi/1.0 (ops@example.com)", r.Header.Get("User-Agent"))
		w.Header().Set("Expires", time.Now().Add(time.Hour).UTC().Format(time.RFC1123))
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	result, err := weather.NewClient(srv.URL, "skynapi/1.0 (ops@example.com)").
		Fetch(context.Background(), 52.3676, 4.9041, weather.FetchOptions{})
	require.NoError(t, err)

	assert.JSONEq(t, body, string(result.Raw))
	assert.Contains(t, string(result.Raw), "future_field", "unmodelled fields must not be dropped")
	assert.NotNil(t, result.ExpiresAt)
}

func TestMETClient_ConditionalRequest(t *testing.T) {
	lastModified := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)

	var gotIMS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIMS = r.Header.Get("If-Modified-Since")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	result, err := weather.NewClient(srv.URL, "test").
		Fetch(context.Background(), 0, 0, weather.FetchOptions{IfModifiedSince: &lastModified})
	require.NoError(t, err)

	assert.True(t, result.NotModified)
	assert.Nil(t, result.Raw, "a 304 carries no body")
	assert.Equal(t, lastModified.Format(time.RFC1123), gotIMS)
}

// An oversized or malformed upstream response must be rejected rather than
// cached and then served behind a JSON content type.
func TestMETClient_RejectsUnusableResponses(t *testing.T) {
	t.Run("body over the size cap", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// 5 MiB of valid JSON, past the 4 MiB ceiling.
			_, _ = w.Write([]byte(`{"padding":"` + strings.Repeat("x", 5<<20) + `"}`))
		}))
		defer srv.Close()

		_, err := weather.NewClient(srv.URL, "test").Fetch(context.Background(), 0, 0, weather.FetchOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds")
	})

	t.Run("malformed JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"truncated":`))
		}))
		defer srv.Close()

		_, err := weather.NewClient(srv.URL, "test").Fetch(context.Background(), 0, 0, weather.FetchOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "malformed JSON")
	})

	t.Run("upstream rate limit", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()

		_, err := weather.NewClient(srv.URL, "test").Fetch(context.Background(), 0, 0, weather.FetchOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "429")
	})
}
