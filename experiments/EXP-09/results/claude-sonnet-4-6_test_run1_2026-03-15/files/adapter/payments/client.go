package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/example/orders/port"
)

type chargeRequest struct {
	CustomerID  string `json:"customer_id"`
	AmountCents int64  `json:"amount_cents"`
	OrderID     string `json:"order_id"`
}

// Client implements port.PaymentService against the payments HTTP API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs a payments Client.
func NewClient(baseURL string, httpClient *http.Client) port.PaymentService {
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

// Charge posts a charge request to the payments API.
func (c *Client) Charge(ctx context.Context, in port.ChargeInput) error {
	body, err := json.Marshal(chargeRequest{
		CustomerID:  in.CustomerID,
		AmountCents: in.AmountCents,
		OrderID:     in.OrderID,
	})
	if err != nil {
		return fmt.Errorf("payments: marshal charge request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/charge", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("payments: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("payments: charge request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("payments: unexpected status %d", resp.StatusCode)
	}

	return nil
}