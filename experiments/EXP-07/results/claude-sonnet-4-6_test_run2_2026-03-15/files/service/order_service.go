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
		FinalCents: in.TotalCents,
		Status:     domain.StatusPending,
		CreatedAt:  time.Now().UTC(),
	}

	if err := order.Validate(); err != nil {
		return nil, err
	}

	if in.DiscountCode != "" {
		if err := s.applyDiscount(ctx, order, in.DiscountCode); err != nil {
			return nil, err
		}
	}

	if err := s.repo.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("save order: %w", err)
	}

	return order, nil
}

func (s *orderService) applyDiscount(ctx context.Context, order *domain.Order, code string) error {
	disc, err := s.discRepo.FindByCode(ctx, code)
	if err != nil {
		if errors.Is(err, domain.ErrDiscountNotFound) {
			return fmt.Errorf("%w: %s", domain.ErrDiscountNotFound, code)
		}
		return fmt.Errorf("find discount: %w", err)
	}

	order.DiscountCode = disc.Code
	order.DiscountPct = disc.Percentage
	order.FinalCents = disc.Apply(order.TotalCents)
	return nil
}

func (s *orderService) GetOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find order: %w", err)
	}
	return order, nil
}