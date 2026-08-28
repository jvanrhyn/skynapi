package geocode_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jvanrhyn/skynapi/internal/geocode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSvc struct {
	place *geocode.Place
	err   error
}

func (f *fakeSvc) Reverse(context.Context, float64, float64) (*geocode.Place, error) {
	return f.place, f.err
}

func route(svc geocode.Service) http.Handler {
	r := chi.NewRouter()
	geocode.NewHandler(svc).RegisterRoutes(r)
	return r
}

func TestHandler_Reverse(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		svc        geocode.Service
		wantStatus int
		wantBody   string
	}{
		{
			name:       "resolved place",
			url:        "/reverse?lat=-26.2041&lon=28.0473",
			svc:        &fakeSvc{place: &geocode.Place{Label: "Johannesburg, South Africa", CountryCode: "ZA"}},
			wantStatus: http.StatusOK,
			wantBody:   "Johannesburg, South Africa",
		},
		{
			name:       "missing both coordinates",
			url:        "/reverse",
			svc:        &fakeSvc{},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "lat and lon are required",
		},
		{
			name:       "unparseable coordinate",
			url:        "/reverse?lat=abc&lon=28.0473",
			svc:        &fakeSvc{},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "must be valid decimal numbers",
		},
		{
			name:       "upstream unavailable",
			url:        "/reverse?lat=-26.2041&lon=28.0473",
			svc:        &fakeSvc{err: geocode.ErrUnavailable},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "temporarily unavailable",
		},
		{
			name:       "unexpected error is not leaked to the client",
			url:        "/reverse?lat=-26.2041&lon=28.0473",
			svc:        &fakeSvc{err: errors.New("connection string with a password in it")},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "internal server error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			route(tc.svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantBody)
			require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

			if tc.wantStatus == http.StatusInternalServerError {
				assert.NotContains(t, rec.Body.String(), "password",
					"internal error detail must not reach the client")
			}
		})
	}
}

// A place name for a rounded coordinate is effectively static, so the response
// should be cacheable.
func TestHandler_SetsCacheControlOnSuccess(t *testing.T) {
	rec := httptest.NewRecorder()
	svc := &fakeSvc{place: &geocode.Place{Label: "Johannesburg, South Africa"}}
	route(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reverse?lat=-26.2&lon=28.05", nil))

	assert.Equal(t, "public, max-age=86400", rec.Header().Get("Cache-Control"))
}
