```go
package port

import "context"

// ChargeInput carries the data required to charge a customer.
type ChargeInput struct {
	OrderID     string
	CustomerID  string
	AmountCents int64
}

// ChargeResult holds the payment provider's response.
type ChargeResult struct {
	TransactionID string
}

// PaymentGateway defines the outbound port for charging customers.
// Implementations live in adapter/payment.
type PaymentGateway interface {
	Charge(ctx context.Context, in ChargeInput) (*ChargeResult, error)
}
```