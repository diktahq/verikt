// Package handler is the HTTP adapter — it drives the inbound port.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/diktahq/verikt/internal/engineclient/experiment/testdata/conforming-hexagonal/domain"
	"github.com/diktahq/verikt/internal/engineclient/experiment/testdata/conforming-hexagonal/port"
)

// OrderHandler handles HTTP requests for orders.
type OrderHandler struct {
	useCase port.OrderUseCase
}

// New returns a new OrderHandler.
func New(useCase port.OrderUseCase) *OrderHandler {
	return &OrderHandler{useCase: useCase}
}

// Create handles POST /orders.
func (h *OrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct{ Customer string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	order, err := h.useCase.CreateOrder(req.Customer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(order)
}

// Get handles GET /orders/{id}.
func (h *OrderHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := domain.OrderID(r.URL.Query().Get("id"))
	order, err := h.useCase.GetOrder(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(order)
}
