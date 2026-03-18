```go
package domain

import "errors"

// ErrDiscountNotFound is returned when a discount code cannot be located.
var ErrDiscountNotFound = errors.New("discount not found")

// Discount represents a promotional code with a percentage reduction.
type Discount struct {
	Code       string
	Percentage int // 1–100
}
```