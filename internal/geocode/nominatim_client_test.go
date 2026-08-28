package geocode_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jvanrhyn/skynapi/internal/geocode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Sending an identifying User-Agent is the reason this call is proxied at all —
// a browser cannot set one, and Nominatim's usage policy requires it.
func TestClient_SendsIdentifyingUserAgent(t *testing.T) {
	var gotUA, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"address":{"city":"Johannesburg","country":"South Africa","country_code":"za"}}`))
	}))
	defer srv.Close()

	client := geocode.NewClient(srv.URL, "skynapi/1.0 (ops@example.com)")
	place, err := client.Reverse(context.Background(), -26.2041, 28.0473)
	require.NoError(t, err)

	assert.Equal(t, "skynapi/1.0 (ops@example.com)", gotUA)
	assert.Contains(t, gotQuery, "lat=-26.2041")
	assert.Contains(t, gotQuery, "lon=28.0473")
	assert.Equal(t, "Johannesburg, South Africa", place.Label)
	assert.Equal(t, "ZA", place.CountryCode)
}

// Nominatim names the settlement field differently by place type; the first
// populated one wins.
func TestClient_FallsBackThroughSettlementFields(t *testing.T) {
	tests := []struct {
		name, body, wantCity string
	}{
		{"town", `{"address":{"town":"Stellenbosch","country":"South Africa"}}`, "Stellenbosch"},
		{"village", `{"address":{"village":"Greyton","country":"South Africa"}}`, "Greyton"},
		{"county falls through", `{"address":{"county":"Overberg","country":"South Africa"}}`, "Overberg"},
		{"city wins over town", `{"address":{"city":"Cape Town","town":"Ignored","country":"South Africa"}}`, "Cape Town"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			place, err := geocode.NewClient(srv.URL, "test").Reverse(context.Background(), 0, 0)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCity, place.City)
		})
	}
}

func TestClient_ErrorPaths(t *testing.T) {
	t.Run("non-200 is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()

		_, err := geocode.NewClient(srv.URL, "test").Reverse(context.Background(), 0, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "429")
	})

	t.Run("a response with no usable name is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"address":{}}`))
		}))
		defer srv.Close()

		_, err := geocode.NewClient(srv.URL, "test").Reverse(context.Background(), 0, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no usable place name")
	})

	t.Run("malformed JSON is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}))
		defer srv.Close()

		_, err := geocode.NewClient(srv.URL, "test").Reverse(context.Background(), 0, 0)
		require.Error(t, err)
	})

	t.Run("trailing slash in base URL is tolerated", func(t *testing.T) {
		var gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			_, _ = w.Write([]byte(`{"address":{"city":"Durban","country":"South Africa"}}`))
		}))
		defer srv.Close()

		_, err := geocode.NewClient(srv.URL+"/", "test").Reverse(context.Background(), 0, 0)
		require.NoError(t, err)
		assert.Equal(t, "/reverse", gotPath)
		assert.False(t, strings.Contains(gotPath, "//"))
	})
}
