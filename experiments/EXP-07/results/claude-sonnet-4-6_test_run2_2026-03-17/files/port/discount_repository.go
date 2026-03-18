```go
package port

import (
	"context"

	"github.com/example/orders/domain"
)

type DiscountRepository interface {
	FindByCode(ctx context.Context, code string) (*domain.Discount, error)
}
```