package port

import "context"

// ChargeInput carries the data required to charge a customer.
type ChargeInput struct {
	CustomerID  string
	AmountCents int64
	OrderID     string
}

// ChargeResult holds the outcome of a successful charge.
type ChargeResult struct {
	TransactionID string
}

// PaymentGateway defines the contract for charging customers.
type PaymentGateway interface {
	Charge(ctx context.Context, in ChargeInput) (*ChargeResult, error)
}