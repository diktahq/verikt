```go
package port

import (
	"context"

	"github.com/example/orders/domain"
)

// DiscountRepository defines lookup operations for discount codes.
type DiscountRepository interface {
	FindByCode(ctx context.Context, code string) (*domain.Discount, error)
}
```