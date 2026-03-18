```go
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/example/orders/domain"
	"github.com/example/orders/port"
)

type orderService struct {
	repo     port.OrderRepository
	discRepo port.DiscountRepository
}

// NewOrderService constructs an OrderService backed by the given repositories.
func NewOrderService(repo port.OrderRepository, discRepo port.DiscountRepository) port.OrderService {
	return &orderService{repo: repo, discRepo: discRepo}
}

func (s *orderService) CreateOrder(ctx context.Context, in port.CreateOrderInput) (*domain.Order, error) {
	total := in.TotalCents
	var discountCode string
	var discountPct int

	if in.DiscountCode != "" {
		disc, err := s.discRepo.FindByCode(ctx, in.DiscountCode)
		if err != nil {
			if errors.Is(err, domain.ErrDiscountNotFound) {
				return nil, fmt.Errorf("%w: %s", domain.ErrDiscountNotFound, in.DiscountCode)
			}
			return nil, fmt.Errorf("lookup discount: %w", err)
		}
		total = disc.Apply(total)
		discountCode = disc.Code
		discountPct = disc.Percentage
	}

	order := &domain.Order{
		ID:           fmt.Sprintf("order-%d", time.Now().UnixNano()),
		CustomerID:   in.CustomerID,
		TotalCents:   total,
		DiscountCode: discountCode,
		DiscountPct:  discountPct,
		Status:       domain.StatusPending,
		CreatedAt:    time.Now().UTC(),
	}

	if err := order.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("save order: %w", err)
	}

	return order, nil
}

func (s *orderService) GetOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find order: %w", err)
	}
	return order, nil
}
```