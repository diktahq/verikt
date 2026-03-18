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
	repo         port.OrderRepository
	discountRepo port.DiscountRepository
}

func NewOrderService(repo port.OrderRepository, discountRepo port.DiscountRepository) port.OrderService {
	return &orderService{repo: repo, discountRepo: discountRepo}
}

func (s *orderService) CreateOrder(ctx context.Context, in port.CreateOrderInput) (*domain.Order, error) {
	order := &domain.Order{
		ID:         fmt.Sprintf("order-%d", time.Now().UnixNano()),
		CustomerID: in.CustomerID,
		TotalCents: in.TotalCents,
		Status:     domain.StatusPending,
		CreatedAt:  time.Now().UTC(),
	}

	if in.DiscountCode != "" {
		discount, err := s.discountRepo.FindByCode(ctx, in.DiscountCode)
		if err != nil {
			if errors.Is(err, domain.ErrDiscountNotFound) {
				return nil, errors.Join(domain.ErrInvalidOrder, domain.ErrDiscountNotFound)
			}
			return nil, fmt.Errorf("find discount: %w", err)
		}
		order.DiscountCode = discount.Code
		order.DiscountPercent = discount.Percentage
		order.TotalCents = order.TotalCents * int64(100-discount.Percentage) / 100
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