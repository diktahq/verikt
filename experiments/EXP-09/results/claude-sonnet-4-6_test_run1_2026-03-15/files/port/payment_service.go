package port

import "context"

// ChargeInput carries the data required to charge a customer.
type ChargeInput struct {
	CustomerID string
	AmountCents int64
	OrderID     string
}

// PaymentService defines the application boundary for payment processing.
type PaymentService interface {
	Charge(ctx context.Context, in ChargeInput) error
}