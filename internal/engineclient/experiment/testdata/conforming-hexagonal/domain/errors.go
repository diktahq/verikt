package domain

import "errors"

// ErrEmptyCustomer is returned when a customer name is blank.
var ErrEmptyCustomer = errors.New("customer name must not be empty")

// ErrOrderNotFound is returned when an order cannot be located.
var ErrOrderNotFound = errors.New("order not found")
