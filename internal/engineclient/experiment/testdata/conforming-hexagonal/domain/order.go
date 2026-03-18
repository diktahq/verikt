// Package domain contains the core business entities and rules.
// It has no outward dependencies — nothing outside this package may be imported.
package domain

// Order is the aggregate root for an order.
type Order struct {
	ID       string
	Customer string
	Total    float64
}

// OrderID is a typed identifier — avoids primitive obsession.
type OrderID string

// NewOrder creates a new Order with validated inputs.
func NewOrder(id OrderID, customer string) (*Order, error) {
	if customer == "" {
		return nil, ErrEmptyCustomer
	}
	return &Order{ID: string(id), Customer: customer}, nil
}
