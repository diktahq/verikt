```go
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/example/orders/domain"
	"github.com/example/orders/port"
)

// Handler holds the HTTP handlers for the orders API.
type Handler struct {
	svc port.OrderService
}

// NewHandler constructs a Handler wired to the given service.
func NewHandler(svc port.OrderService) *Handler {
	return &Handler{svc: svc}
}

type createOrderRequest struct {
	CustomerID   string `json:"customer_id"`
	TotalCents   int64  `json:"total_cents"`
	DiscountCode string `json:"discount_code,omitempty"`
}

type orderResponse struct {
	ID           string `json:"id"`
	CustomerID   string `json:"customer_id"`
	TotalCents   int64  `json:"total_cents"`
	DiscountCode string `json:"discount_code,omitempty"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
}

func toResponse(o *domain.Order) orderResponse {
	return orderResponse{
		ID:           o.ID,
		CustomerID:   o.CustomerID,
		TotalCents:   o.TotalCents,
		DiscountCode: o.DiscountCode,
		Status:       string(o.Status),
		CreatedAt:    o.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// CreateOrder handles POST /orders.
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	order, err := h.svc.CreateOrder(r.Context(), port.CreateOrderInput{
		CustomerID:   req.CustomerID,
		TotalCents:   req.TotalCents,
		DiscountCode: req.DiscountCode,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidOrder) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toResponse(order)) //nolint:errcheck
}

// GetOrder handles GET /orders/{id}.
func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing order id", http.StatusBadRequest)
		return
	}

	order, err := h.svc.GetOrder(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			http.Error(w, "order not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toResponse(order)) //nolint:errcheck
}
```