```go
package port

import "context"

// ChargeInput carries the data required to charge a customer.
type ChargeInput struct {
	CustomerID string
	AmountCents int64
	OrderID    string
}

// ChargeResult holds the response from a successful charge.
type ChargeResult struct {
	ChargeID string
}

// PaymentGateway defines the contract for charging customers.
// The adapter in adapter/payments implements this against the real API.
type PaymentGateway interface {
	Charge(ctx context.Context, in ChargeInput) (*ChargeResult, error)
}
```