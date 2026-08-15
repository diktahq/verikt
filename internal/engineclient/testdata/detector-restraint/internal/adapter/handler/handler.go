package handler

import "net/http"

// Handle uses the request context, not context.Background().
func Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = ctx
	w.WriteHeader(http.StatusOK)
}
