package city

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-playground/validator/v10"
)

// Service abstracts city search business logic.
type Service interface {
	Search(ctx context.Context, params SearchParams) (*SearchResult, error)
}

type service struct {
	repo     Repository
	validate *validator.Validate
}

// NewService returns a Service. It is safe for concurrent use.
func NewService(repo Repository) Service {
	return &service{
		repo:     repo,
		validate: validator.New(),
	}
}

func (s *service) Search(ctx context.Context, params SearchParams) (*SearchResult, error) {
	// Apply defaults before validation.
	if params.Page == 0 {
		params.Page = 1
	}
	if params.Limit == 0 {
		params.Limit = 20
	}

	if err := s.validate.Struct(params); err != nil {
		return nil, fmt.Errorf("city: invalid params: %w", err)
	}

	cities, total, err := s.repo.Search(ctx, params)
	if err != nil {
		slog.ErrorContext(ctx, "city search failed", "error", err, "q", params.Q)
		return nil, fmt.Errorf("city: search: %w", err)
	}

	if cities == nil {
		cities = []City{}
	}

	return &SearchResult{
		Cities: cities,
		Total:  total,
		Page:   params.Page,
		Limit:  params.Limit,
	}, nil
}
