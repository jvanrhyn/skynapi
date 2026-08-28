package weather_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jvanrhyn/skynapi/internal/weather"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHandler_ETagRoundTrip(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	result := &weather.WeatherResult{
		Data:      json.RawMessage(`{"type":"Feature","properties":{}}`),
		ExpiresAt: &expires,
		Source:    "cache",
	}

	svc := &mockWeatherSvc{}
	svc.On("GetWeather", mock.Anything, mock.Anything, mock.Anything).Return(result, nil)
	h := setupWeatherRouter(svc)

	// First read returns the body and an ETag.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/weather?lat=52.3676&lon=4.9041", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	etag := rec.Header().Get("ETag")
	require.NotEmpty(t, etag)
	assert.Contains(t, rec.Body.String(), "Feature")

	// A repeat read presenting that ETag gets a 304 with no body.
	req := httptest.NewRequest(http.MethodGet, "/weather?lat=52.3676&lon=4.9041", nil)
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotModified, rec.Code)
	assert.Empty(t, rec.Body.String())

	// A stale validator still gets the full body.
	req = httptest.NewRequest(http.MethodGet, "/weather?lat=52.3676&lon=4.9041", nil)
	req.Header.Set("If-None-Match", `W/"deadbeef"`)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Feature")
}

// Absent coordinates and unparseable coordinates are different mistakes and
// should not produce the same message.
func TestHandler_MissingVersusMalformedCoordinates(t *testing.T) {
	svc := &mockWeatherSvc{}
	h := setupWeatherRouter(svc)

	tests := []struct {
		name, url, wantMessage string
	}{
		{"no parameters at all", "/weather", "lat and lon are required"},
		{"only lat supplied", "/weather?lat=52.3676", "lat and lon are required"},
		{"empty lon value", "/weather?lat=52.3676&lon=", "lat and lon are required"},
		{"unparseable lat", "/weather?lat=abc&lon=4.9041", "must be valid decimal numbers"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))
			assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantMessage)
		})
	}

	svc.AssertNotCalled(t, "GetWeather", mock.Anything, mock.Anything, mock.Anything)
}
