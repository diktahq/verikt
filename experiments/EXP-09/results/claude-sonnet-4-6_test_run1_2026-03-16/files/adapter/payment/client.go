```go
package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/example/orders/port"
)

const defaultTimeout = 10 * time.Second

// Client implements port.PaymentGateway against the payments HTTP API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs a payment Client targeting baseURL.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
	}
}

type chargeRequest struct {
	OrderID     string `json:"order_id"`
	CustomerID  string `json:"customer_id"`
	AmountCents int64  `json:"amount_cents"`
}

type chargeResponse struct {
	TransactionID string `json:"transaction_id"`
}

// Charge posts a charge to the payment API and returns the transaction ID.
func (c *Client) Charge(ctx context.Context, in port.ChargeInput) (*port.ChargeResult, error) {
	payload, err := json.Marshal(chargeRequest{
		OrderID:     in.OrderID,
		CustomerID:  in.CustomerID,
		AmountCents: in.AmountCents,
	})
	if err != nil {
		return nil, fmt.Errorf("payment charge marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/charge", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("payment charge build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("payment charge: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("payment charge: unexpected status %d", resp.StatusCode)
	}

	var result chargeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("payment charge decode response: %w", err)
	}

	return &port.ChargeResult{TransactionID: result.TransactionID}, nil
}
```