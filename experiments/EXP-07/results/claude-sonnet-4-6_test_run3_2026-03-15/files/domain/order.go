package domain

import (
	"errors"
	"time"
)

// OrderStatus represents the lifecycle state of an order.
type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusConfirmed OrderStatus = "confirmed"
	StatusShipped   OrderStatus = "shipped"
	StatusCancelled OrderStatus = "cancelled"
)

// ErrOrderNotFound is returned when an order cannot be located by ID.
var ErrOrderNotFound = errors.New("order not found")

// ErrInvalidOrder is returned when an order fails domain validation.
var ErrInvalidOrder = errors.New("invalid order")

// Order is the central aggregate of the ordering domain.
type Order struct {
	ID              string
	CustomerID      string
	TotalCents      int64
	DiscountCode    string
	DiscountPercent int
	Status          OrderStatus
	CreatedAt       time.Time
}

// Validate checks that the order satisfies domain invariants.
// Returns ErrInvalidOrder if any invariant is violated.
func (o *Order) Validate() error {
	if o.CustomerID == "" {
		return errors.Join(ErrInvalidOrder, errors.New("customer ID must not be empty"))
	}
	if o.TotalCents <= 0 {
		return errors.Join(ErrInvalidOrder, errors.New("total must be greater than zero"))
	}
	return nil
}