package port

import (
	"context"

	"github.com/example/orders/domain"
)

// CreateOrderInput carries the data required to place a new order.
type CreateOrderInput struct {
	CustomerID   string
	TotalCents   int64
	DiscountCode string // optional
}

// OrderService defines the application use cases for order management.
// The HTTP adapter and any future transport call into this interface.
type OrderService interface {
	CreateOrder(ctx context.Context, in CreateOrderInput) (*domain.Order, error)
	GetOrder(ctx context.Context, id string) (*domain.Order, error)
}