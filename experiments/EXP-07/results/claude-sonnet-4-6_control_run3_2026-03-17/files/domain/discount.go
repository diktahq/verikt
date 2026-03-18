```go
package domain

import "errors"

// ErrDiscountNotFound is returned when a discount code cannot be located.
var ErrDiscountNotFound = errors.New("discount not found")

// Discount represents a promotional code with a fixed percentage off.
type Discount struct {
	Code       string
	Percentage int // 1–100
}

// Apply reduces the given amount by the discount percentage, rounding down.
func (d *Discount) Apply(totalCents int64) int64 {
	return totalCents * int64(100-d.Percentage) / 100
}
```