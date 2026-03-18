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

	if in.DiscountCode != "" {
		discount, err := s.discRepo.FindByCode(ctx, in.DiscountCode)
		if err != nil && !errors.Is(err, domain.ErrDiscountNotFound) {
			return nil, fmt.Errorf("look up discount: %w", err)
		}
		if errors.Is(err, domain.ErrDiscountNotFound) {
			return nil, fmt.Errorf("%w: discount code %q not found", domain.ErrInvalidOrder, in.DiscountCode)
		}
		order.ApplyDiscount(*discount)
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