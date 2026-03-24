// Package service implements application use cases.
// It may import domain and port — it must not import adapter.
package service

import (
	"github.com/diktahq/verikt/internal/engineclient/experiment/testdata/conforming-hexagonal/domain"
	"github.com/diktahq/verikt/internal/engineclient/experiment/testdata/conforming-hexagonal/port"
)

// OrderService implements port.OrderUseCase.
type OrderService struct {
	repo port.OrderRepository
}

// New returns a new OrderService.
func New(repo port.OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

// CreateOrder creates and persists a new order.
func (s *OrderService) CreateOrder(customerName string) (*domain.Order, error) {
	order, err := domain.NewOrder(domain.OrderID("gen-id"), customerName)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Save(order); err != nil {
		return nil, err
	}
	return order, nil
}

// GetOrder retrieves an order by ID.
func (s *OrderService) GetOrder(id domain.OrderID) (*domain.Order, error) {
	return s.repo.FindByID(id)
}
