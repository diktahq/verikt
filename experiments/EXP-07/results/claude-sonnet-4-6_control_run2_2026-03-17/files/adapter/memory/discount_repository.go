package memory

import (
	"context"
	"sync"

	"github.com/example/orders/domain"
	"github.com/example/orders/port"
)

type discountRepository struct {
	mu        sync.RWMutex
	discounts map[string]domain.Discount
}

// NewDiscountRepository returns an in-memory DiscountRepository seeded with
// the provided discounts.
func NewDiscountRepository(discounts []domain.Discount) port.DiscountRepository {
	m := make(map[string]domain.Discount, len(discounts))
	for _, d := range discounts {
		m[d.Code] = d
	}
	return &discountRepository{discounts: m}
}

func (r *discountRepository) FindByCode(ctx context.Context, code string) (*domain.Discount, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.discounts[code]
	if !ok {
		return nil, domain.ErrDiscountNotFound
	}
	return &d, nil
}