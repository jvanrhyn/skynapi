package city

import "context"

// Repository abstracts all persistence operations for city search.
// Implementations must be safe for concurrent use.
type Repository interface {
	// Search returns cities matching params and the total count of matching rows.
	Search(ctx context.Context, params SearchParams) (cities []City, total int, err error)
}
