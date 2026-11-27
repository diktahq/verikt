package handler

import (
	"context"
	"net/http"
)

// Handle has the request in scope and drops its cancellation: the lookup runs
// on after the client has gone.
func Handle(w http.ResponseWriter, r *http.Request) {
	_ = lookup(context.Background(), r.URL.Path)
	w.WriteHeader(http.StatusOK)
}

func lookup(ctx context.Context, key string) error {
	_ = key
	return ctx.Err()
}
