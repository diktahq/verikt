```go
package payments

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

// HTTPClient implements port.PaymentGateway against the payments HTTP API.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPClient constructs an HTTPClient targeting the given base URL.
func NewHTTPClient(baseURL string) port.PaymentGateway {
	return &HTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

type chargeRequest struct {
	OrderID     string `json:"order_id"`
	CustomerID  string `json:"customer_id"`
	AmountCents int64  `json:"amount_cents"`
}

type chargeResponse struct {
	ChargeID string `json:"charge_id"`
}

// Charge sends a POST /charge request to the payments API.
func (c *HTTPClient) Charge(ctx context.Context, in port.ChargeInput) (*port.ChargeResult, error) {
	body, err := json.Marshal(chargeRequest{
		OrderID:     in.OrderID,
		CustomerID:  in.CustomerID,
		AmountCents: in.AmountCents,
	})
	if err != nil {
		return nil, fmt.Errorf("payments charge: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/charge", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("payments charge: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("payments charge: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("payments charge: unexpected status %d", resp.StatusCode)
	}

	var result chargeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("payments charge: decode response: %w", err)
	}

	return &port.ChargeResult{ChargeID: result.ChargeID}, nil
}
```