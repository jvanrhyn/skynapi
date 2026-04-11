package weather_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jvanrhyn/skynapi/internal/weather"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockWeatherSvc satisfies weather.Service for handler tests.
type mockWeatherSvc struct{ mock.Mock }

func (m *mockWeatherSvc) GetWeather(ctx context.Context, lat, lon float64) (json.RawMessage, error) {
	args := m.Called(ctx, lat, lon)
	raw, _ := args.Get(0).(json.RawMessage)
	return raw, args.Error(1)
}

func setupWeatherRouter(svc weather.Service) http.Handler {
	r := chi.NewRouter()
	h := weather.NewHandler(svc)
	h.RegisterRoutes(r)
	return r
}

func TestHandler_GetWeather(t *testing.T) {
	sampleData := json.RawMessage(`{"type":"Feature","properties":{}}`)

	tests := []struct {
		name           string
		url            string
		setupMock      func(*mockWeatherSvc)
		wantStatus     int
		wantBodyContains string
	}{
		{
			name: "successful forecast",
			url:  "/weather?lat=52.3676&lon=4.9041",
			setupMock: func(m *mockWeatherSvc) {
				m.On("GetWeather", mock.Anything, 52.3676, 4.9041).Return(sampleData, nil)
			},
			wantStatus:       http.StatusOK,
			wantBodyContains: "Feature",
		},
		{
			name:             "missing lat returns 422",
			url:              "/weather?lon=4.9041",
			setupMock:        func(m *mockWeatherSvc) {},
			wantStatus:       http.StatusUnprocessableEntity,
			wantBodyContains: "lat and lon",
		},
		{
			name:             "missing lon returns 422",
			url:              "/weather?lat=52.3676",
			setupMock:        func(m *mockWeatherSvc) {},
			wantStatus:       http.StatusUnprocessableEntity,
			wantBodyContains: "lat and lon",
		},
		{
			name:             "non-numeric lat returns 422",
			url:              "/weather?lat=abc&lon=4.9041",
			setupMock:        func(m *mockWeatherSvc) {},
			wantStatus:       http.StatusUnprocessableEntity,
		},
		{
			name: "upstream unavailable returns 503",
			url:  "/weather?lat=52.3676&lon=4.9041",
			setupMock: func(m *mockWeatherSvc) {
				m.On("GetWeather", mock.Anything, 52.3676, 4.9041).Return(nil, weather.ErrUpstreamUnavailable)
			},
			wantStatus:       http.StatusServiceUnavailable,
			wantBodyContains: "unavailable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockWeatherSvc{}
			tc.setupMock(svc)

			router := setupWeatherRouter(svc)
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			if tc.wantBodyContains != "" {
				assert.Contains(t, rec.Body.String(), tc.wantBodyContains)
			}
			svc.AssertExpectations(t)
		})
	}
}

