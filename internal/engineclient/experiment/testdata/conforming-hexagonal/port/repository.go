// Package port defines the inbound and outbound interfaces (ports).
// It may import domain — it must not import service or adapter.
package port

import "github.com/diktahq/verikt/internal/engineclient/experiment/testdata/conforming-hexagonal/domain"

// OrderRepository is the outbound port for order persistence.
type OrderRepository interface {
	Save(order *domain.Order) error
	FindByID(id domain.OrderID) (*domain.Order, error)
}

// OrderUseCase is the inbound port — driven by adapters.
type OrderUseCase interface {
	CreateOrder(customerName string) (*domain.Order, error)
	GetOrder(id domain.OrderID) (*domain.Order, error)
}
