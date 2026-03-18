```go
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/example/orders/domain"
	"github.com/example/orders/port"
)

type orderService struct {
	repo      port.OrderRepository
	discounts port.DiscountRepository
}

// NewOrderService constructs an OrderService backed by the given repositories.
func NewOrderService(repo port.OrderRepository, discounts port.DiscountRepository) port.OrderService {
	return &orderService{repo: repo, discounts: discounts}
}

func (s *orderService) CreateOrder(ctx context.Context, in port.CreateOrderInput) (*domain.Order, error) {
	total, discountCode, err := s.applyDiscount(ctx, in.TotalCents, in.DiscountCode)
	if err != nil {
		return nil, err
	}

	order := &domain.Order{
		ID:           fmt.Sprintf("order-%d", time.Now().UnixNano()),
		CustomerID:   in.CustomerID,
		TotalCents:   total,
		DiscountCode: discountCode,
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

func (s *orderService) applyDiscount(ctx context.Context, totalCents int64, code string) (int64, string, error) {
	if code == "" {
		return totalCents, "", nil
	}

	discount, err := s.discounts.FindByCode(ctx, code)
	if err != nil {
		return 0, "", fmt.Errorf("look up discount: %w", err)
	}

	return discount.Apply(totalCents), discount.Code, nil
}

func (s *orderService) GetOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find order: %w", err)
	}
	return order, nil
}
```