// Package memory provides an in-memory implementation of port.OrderRepository.
// Adapters may import domain, port, and service — never the reverse.
package memory

import (
	"github.com/diktahq/verikt/internal/engineclient/experiment/testdata/conforming-hexagonal/domain"
	"github.com/diktahq/verikt/internal/engineclient/experiment/testdata/conforming-hexagonal/port"
)

// Repo is an in-memory order repository.
type Repo struct {
	store map[domain.OrderID]*domain.Order
}

// NewRepo returns an initialised in-memory Repo.
func NewRepo() port.OrderRepository {
	return &Repo{store: make(map[domain.OrderID]*domain.Order)}
}

// Save stores the order.
func (r *Repo) Save(order *domain.Order) error {
	r.store[domain.OrderID(order.ID)] = order
	return nil
}

// FindByID retrieves an order or returns ErrOrderNotFound.
func (r *Repo) FindByID(id domain.OrderID) (*domain.Order, error) {
	order, ok := r.store[id]
	if !ok {
		return nil, domain.ErrOrderNotFound
	}
	return order, nil
}
