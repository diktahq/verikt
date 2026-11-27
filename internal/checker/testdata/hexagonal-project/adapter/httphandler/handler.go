package httphandler

import (
	"context"
	"net/http"
)

type Handler struct{}

// HandleRequest passes context.Background() downstream with the request in
// scope — should trigger context_background_in_handler.
func (h *Handler) HandleRequest(w http.ResponseWriter, r *http.Request) {
	process(context.Background())
	w.WriteHeader(http.StatusOK)

	// Naked goroutine — should trigger naked_goroutine.
	go func() {
		println("background work")
	}()
}

func process(ctx context.Context) {
	_ = ctx
}
