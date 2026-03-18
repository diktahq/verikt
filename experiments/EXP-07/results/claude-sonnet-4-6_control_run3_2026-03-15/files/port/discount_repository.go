```go
package port

import (
	"context"

	"github.com/example/orders/domain"
)

// DiscountRepository defines persistence operations for discounts.
type DiscountRepository interface {
	FindByCode(ctx context.Context, code string) (*domain.Discount, error)
}
```