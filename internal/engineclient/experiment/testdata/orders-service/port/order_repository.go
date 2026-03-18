package port

import (
	"context"

	"github.com/example/orders/domain"
)

// OrderRepository defines persistence operations for orders.
// Implementations live in adapter/postgres or any other backing store.
type OrderRepository interface {
	Save(ctx context.Context, order *domain.Order) error
	FindByID(ctx context.Context, id string) (*domain.Order, error)
}
