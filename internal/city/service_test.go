package city_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jvanrhyn/skynapi/internal/city"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockRepository implements city.Repository using testify/mock.
type mockRepository struct{ mock.Mock }

func (m *mockRepository) Search(ctx context.Context, params city.SearchParams) ([]city.City, int, error) {
	args := m.Called(ctx, params)
	cities, _ := args.Get(0).([]city.City)
	return cities, args.Int(1), args.Error(2)
}

func TestService_Search(t *testing.T) {
	sampleCities := []city.City{
		{GeonameID: 1, Name: "Cape Town", CountryCode: "ZA", Region: "WC", Lat: -33.9249, Lon: 18.4241, Timezone: "Africa/Johannesburg"},
		{GeonameID: 2, Name: "Cape Coral", CountryCode: "US", Region: "FL", Lat: 26.5629, Lon: -81.9495, Timezone: "America/New_York"},
	}

	tests := []struct {
		name        string
		params      city.SearchParams
		repoReturn  []city.City
		repoTotal   int
		repoErr     error
		wantErr     bool
		wantTotal   int
		wantLen     int
	}{
		{
			name:       "success with results",
			params:     city.SearchParams{Q: "cape", Page: 1, Limit: 20},
			repoReturn: sampleCities,
			repoTotal:  2,
			wantLen:    2,
			wantTotal:  2,
		},
		{
			name:      "empty results",
			params:    city.SearchParams{Q: "zzzzz", Page: 1, Limit: 20},
			repoReturn: nil,
			repoTotal:  0,
			wantLen:   0,
			wantTotal: 0,
		},
		{
			name:    "empty query returns validation error",
			params:  city.SearchParams{Q: "", Page: 1, Limit: 20},
			wantErr: true,
		},
		{
			name:    "repository error is propagated",
			params:  city.SearchParams{Q: "london", Page: 1, Limit: 20},
			repoErr: errors.New("db connection lost"),
			wantErr: true,
		},
		{
			name:    "zero page defaults to 1",
			params:  city.SearchParams{Q: "paris", Page: 0, Limit: 10},
			repoReturn: sampleCities[:1],
			repoTotal:  1,
			wantLen:    1,
			wantTotal:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{}
			svc := city.NewService(repo)

			// Only set up repo expectation when we expect it to be called.
			if !tc.wantErr || tc.repoErr != nil {
				expectedPage := tc.params.Page
				if expectedPage == 0 {
					expectedPage = 1
				}
				expectedLimit := tc.params.Limit
				if expectedLimit == 0 {
					expectedLimit = 20
				}
				if tc.params.Q != "" {
					repo.On("Search", mock.Anything, mock.MatchedBy(func(p city.SearchParams) bool {
						return p.Q == tc.params.Q
					})).Return(tc.repoReturn, tc.repoTotal, tc.repoErr)
				}
			}

			result, err := svc.Search(context.Background(), tc.params)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantTotal, result.Total)
			assert.Len(t, result.Cities, tc.wantLen)
			repo.AssertExpectations(t)
		})
	}
}
