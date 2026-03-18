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
	repo     port.OrderRepository
	payments port.PaymentGateway
}

// NewOrderService constructs an OrderService backed by the given repository and payment gateway.
func NewOrderService(repo port.OrderRepository, payments port.PaymentGateway) port.OrderService {
	return &orderService{repo: repo, payments: payments}
}

func (s *orderService) CreateOrder(ctx context.Context, in port.CreateOrderInput) (*domain.Order, error) {
	order := &domain.Order{
		ID:         fmt.Sprintf("order-%d", time.Now().UnixNano()),
		CustomerID: in.CustomerID,
		TotalCents: in.TotalCents,
		Status:     domain.StatusPending,
		CreatedAt:  time.Now().UTC(),
	}

	if err := order.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("save order: %w", err)
	}

	if _, err := s.payments.Charge(ctx, port.ChargeInput{
		CustomerID:  order.CustomerID,
		AmountCents: order.TotalCents,
		OrderID:     order.ID,
	}); err != nil {
		return nil, fmt.Errorf("charge customer: %w", err)
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