```go
package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/example/orders/port"
)

const chargeEndpoint = "https://api.payments.example.com/charge"

// HTTPGateway implements port.PaymentGateway over HTTP.
type HTTPGateway struct {
	client  *http.Client
	baseURL string
}

// NewHTTPGateway constructs a gateway pointing at the payments API.
// Pass a custom baseURL in tests to target a stub server.
func NewHTTPGateway(client *http.Client, baseURL string) port.PaymentGateway {
	if baseURL == "" {
		baseURL = chargeEndpoint
	}
	return &HTTPGateway{client: client, baseURL: baseURL}
}

type chargeRequest struct {
	OrderID     string `json:"order_id"`
	CustomerID  string `json:"customer_id"`
	AmountCents int64  `json:"amount_cents"`
}

type chargeResponse struct {
	ChargeID string `json:"charge_id"`
}

func (g *HTTPGateway) Charge(ctx context.Context, in port.ChargeInput) (*port.ChargeResult, error) {
	body, err := json.Marshal(chargeRequest{
		OrderID:     in.OrderID,
		CustomerID:  in.CustomerID,
		AmountCents: in.AmountCents,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal charge request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build charge request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", port.ErrPaymentFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: provider returned %d", port.ErrPaymentFailed, resp.StatusCode)
	}

	var result chargeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode charge response: %w", err)
	}

	return &port.ChargeResult{ChargeID: result.ChargeID}, nil
}
```