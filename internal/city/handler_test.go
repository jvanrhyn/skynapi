package city_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/jvanrhyn/skynapi/internal/city"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// validationErr returns a real validator.ValidationErrors by triggering
// a validation failure on a known-invalid struct.
func validationErr() validator.ValidationErrors {
	type stub struct {
		Q string `validate:"required,min=1"`
	}
	v := validator.New()
	err := v.Struct(stub{Q: ""})
	var ve validator.ValidationErrors
	_ = fmt.Errorf("%w", err) // suppress linter
	_ = ve
	return err.(validator.ValidationErrors)
}

// mockService implements city.Service using testify/mock.
type mockService struct{ mock.Mock }

func (m *mockService) Search(ctx context.Context, params city.SearchParams) (*city.SearchResult, error) {
	args := m.Called(ctx, params)
	res, _ := args.Get(0).(*city.SearchResult)
	return res, args.Error(1)
}

func setupRouter(svc city.Service) http.Handler {
	r := chi.NewRouter()
	h := city.NewHandler(svc)
	h.RegisterRoutes(r)
	return r
}

func TestHandler_Search(t *testing.T) {
	sampleResult := &city.SearchResult{
		Cities: []city.City{
			{GeonameID: 1, Name: "Amsterdam", CountryCode: "NL", Lat: 52.3676, Lon: 4.9041, Timezone: "Europe/Amsterdam"},
		},
		Total: 1, Page: 1, Limit: 20,
	}

	tests := []struct {
		name           string
		url            string
		setupMock      func(*mockService)
		wantStatus     int
		wantBodyContains string
	}{
		{
			name: "successful search",
			url:  "/cities?q=amsterdam",
			setupMock: func(m *mockService) {
				m.On("Search", mock.Anything, mock.MatchedBy(func(p city.SearchParams) bool {
					return p.Q == "amsterdam"
				})).Return(sampleResult, nil)
			},
			wantStatus:       http.StatusOK,
			wantBodyContains: "Amsterdam",
		},
		{
			name: "empty query returns 422",
			url:  "/cities?q=",
			setupMock: func(m *mockService) {
				m.On("Search", mock.Anything, mock.MatchedBy(func(p city.SearchParams) bool {
					return p.Q == ""
				})).Return(nil, fmt.Errorf("city: invalid params: %w", validationErr()))
			},
			wantStatus:       http.StatusUnprocessableEntity,
			wantBodyContains: "validation",
		},
		{
			name: "no q parameter returns 422",
			url:  "/cities",
			setupMock: func(m *mockService) {
				m.On("Search", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("city: invalid params: %w", validationErr()))
			},
			wantStatus:       http.StatusUnprocessableEntity,
			wantBodyContains: "validation",
		},
		{
			name: "empty results returns 200 with empty array",
			url:  "/cities?q=xyzzy",
			setupMock: func(m *mockService) {
				m.On("Search", mock.Anything, mock.Anything).Return(&city.SearchResult{
					Cities: []city.City{}, Total: 0, Page: 1, Limit: 20,
				}, nil)
			},
			wantStatus:       http.StatusOK,
			wantBodyContains: `"total":0`,
		},
		{
			name: "pagination parameters are forwarded",
			url:  "/cities?q=paris&page=2&limit=5",
			setupMock: func(m *mockService) {
				m.On("Search", mock.Anything, mock.MatchedBy(func(p city.SearchParams) bool {
					return p.Q == "paris" && p.Page == 2 && p.Limit == 5
				})).Return(&city.SearchResult{Cities: []city.City{}, Total: 0, Page: 2, Limit: 5}, nil)
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockService{}
			tc.setupMock(svc)

			router := setupRouter(svc)
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			if tc.wantBodyContains != "" {
				assert.Contains(t, rec.Body.String(), tc.wantBodyContains)
			}
			if tc.wantStatus == http.StatusOK {
				var body map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
			}
			svc.AssertExpectations(t)
		})
	}
}
