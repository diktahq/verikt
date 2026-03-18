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
	repo    port.OrderRepository
	payment port.PaymentGateway
}

// NewOrderService constructs an OrderService backed by the given repository and payment gateway.
func NewOrderService(repo port.OrderRepository, payment port.PaymentGateway) port.OrderService {
	return &orderService{repo: repo, payment: payment}
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

	// Persist before charging: if the process crashes between here and the
	// payment call, the pending order is recoverable. An unpersisted charge
	// is not.
	if err := s.repo.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("save order: %w", err)
	}

	result, err := s.payment.Charge(ctx, port.ChargeInput{
		OrderID:     order.ID,
		CustomerID:  order.CustomerID,
		AmountCents: order.TotalCents,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrPaymentFailed, err)
	}

	order.Status = domain.StatusConfirmed
	order.TransactionID = result.TransactionID

	if err := s.repo.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("confirm order: %w", err)
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