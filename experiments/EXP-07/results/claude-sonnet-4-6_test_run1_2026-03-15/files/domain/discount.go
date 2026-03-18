```go
package domain

import "errors"

// ErrDiscountNotFound is returned when a discount code cannot be located.
var ErrDiscountNotFound = errors.New("discount not found")

// ErrInvalidDiscount is returned when a discount fails domain validation.
var ErrInvalidDiscount = errors.New("invalid discount")

// Discount represents a promotional code that reduces an order total by a percentage.
type Discount struct {
	Code       string
	Percentage int // 1–100
}

// Validate checks that the discount satisfies domain invariants.
func (d *Discount) Validate() error {
	if d.Code == "" {
		return errors.Join(ErrInvalidDiscount, errors.New("discount code must not be empty"))
	}
	if d.Percentage < 1 || d.Percentage > 100 {
		return errors.Join(ErrInvalidDiscount, errors.New("discount percentage must be between 1 and 100"))
	}
	return nil
}

// Apply returns the discounted value of amountCents.
func (d *Discount) Apply(amountCents int64) int64 {
	return amountCents - (amountCents * int64(d.Percentage) / 100)
}
```