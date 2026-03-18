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
	discounts port.DiscountRepository
}

// NewOrderService constructs an OrderService backed by the given repositories.
func NewOrderService(repo port.OrderRepository, discounts port.DiscountRepository) port.OrderService {
	return &orderService{repo: repo, discounts: discounts}
}

func (s *orderService) CreateOrder(ctx context.Context, in port.CreateOrderInput) (*domain.Order, error) {
	total := in.TotalCents

	order := &domain.Order{
		ID:         fmt.Sprintf("order-%d", time.Now().UnixNano()),
		CustomerID: in.CustomerID,
		Status:     domain.StatusPending,
		CreatedAt:  time.Now().UTC(),
	}

	if in.DiscountCode != "" {
		discount, err := s.discounts.FindByCode(ctx, in.DiscountCode)
		if err != nil && !errors.Is(err, domain.ErrDiscountNotFound) {
			return nil, fmt.Errorf("look up discount: %w", err)
		}
		if discount != nil {
			total = discount.Apply(total)
			order.DiscountCode = discount.Code
			order.DiscountPercent = discount.Percentage
		}
	}

	order.TotalCents = total

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