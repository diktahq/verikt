package domain

import "errors"

// ErrDiscountNotFound is returned when a discount code cannot be located.
var ErrDiscountNotFound = errors.New("discount not found")

// Discount represents a promotional code that reduces an order total.
type Discount struct {
	Code    string
	Percent int // 1–100
}