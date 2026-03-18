```go
package domain

import (
	"errors"
	"time"
)

type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusConfirmed OrderStatus = "confirmed"
	StatusShipped   OrderStatus = "shipped"
	StatusCancelled OrderStatus = "cancelled"
)

var ErrOrderNotFound = errors.New("order not found")
var ErrInvalidOrder = errors.New("invalid order")

type Order struct {
	ID              string
	CustomerID      string
	TotalCents      int64
	Status          OrderStatus
	CreatedAt       time.Time
	DiscountCode    string
	DiscountPercent int
}

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