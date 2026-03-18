```go
package domain

import (
	"errors"
	"fmt"
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

// ErrOrderNotCancellable is returned when an order cannot be cancelled in its current state.
var ErrOrderNotCancellable = errors.New("order cannot be cancelled")

// Order is the central aggregate of the ordering domain.
type Order struct {
	ID         string
	CustomerID string
	TotalCents int64
	Status     OrderStatus
	CreatedAt  time.Time
}

// Cancel transitions the order to cancelled status.
// Returns ErrOrderNotCancellable if the order has already shipped or been cancelled.
func (o *Order) Cancel() error {
	if o.Status == StatusShipped || o.Status == StatusCancelled {
		return fmt.Errorf("%w: status is %s", ErrOrderNotCancellable, o.Status)
	}
	o.Status = StatusCancelled
	return nil
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
```