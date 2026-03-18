package memory

import (
	"context"
	"sync"

	"github.com/example/orders/domain"
	"github.com/example/orders/port"
)

// DiscountRepository is a thread-safe in-memory discount store.
type DiscountRepository struct {
	mu        sync.RWMutex
	discounts map[string]*domain.Discount
}

// NewDiscountRepository constructs an in-memory DiscountRepository seeded
// with the provided discounts. Useful for development and testing.
func NewDiscountRepository(initial ...*domain.Discount) port.DiscountRepository {
	r := &DiscountRepository{
		discounts: make(map[string]*domain.Discount, len(initial)),
	}
	for _, d := range initial {
		r.discounts[d.Code] = d
	}
	return r
}

// FindByCode returns the discount for the given code or domain.ErrDiscountNotFound.
func (r *DiscountRepository) FindByCode(_ context.Context, code string) (*domain.Discount, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.discounts[code]
	if !ok {
		return nil, domain.ErrDiscountNotFound
	}
	return d, nil
}