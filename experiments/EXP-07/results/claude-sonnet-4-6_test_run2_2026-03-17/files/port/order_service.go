```go
package port

import (
	"context"

	"github.com/example/orders/domain"
)

type CreateOrderInput struct {
	CustomerID   string
	TotalCents   int64
	DiscountCode string
}

type OrderService interface {
	CreateOrder(ctx context.Context, in CreateOrderInput) (*domain.Order, error)
	GetOrder(ctx context.Context, id string) (*domain.Order, error)
}
```