package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/example/orders/port"
)

const chargeEndpoint = "https://api.payments.example.com/charge"

// Client implements port.PaymentGateway over HTTP.
type Client struct {
	http *http.Client
}

// NewClient constructs a payments Client using the given HTTP client.
func NewClient(httpClient *http.Client) port.PaymentGateway {
	return &Client{http: httpClient}
}

type chargeRequest struct {
	CustomerID  string `json:"customer_id"`
	AmountCents int64  `json:"amount_cents"`
	OrderID     string `json:"order_id"`
}

type chargeResponse struct {
	TransactionID string `json:"transaction_id"`
}

// Charge POSTs a charge request to the payments API.
func (c *Client) Charge(ctx context.Context, in port.ChargeInput) (*port.ChargeResult, error) {
	body, err := json.Marshal(chargeRequest{
		CustomerID:  in.CustomerID,
		AmountCents: in.AmountCents,
		OrderID:     in.OrderID,
	})
	if err != nil {
		return nil, fmt.Errorf("payments charge: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chargeEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("payments charge: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
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

	return &port.ChargeResult{TransactionID: result.TransactionID}, nil
}