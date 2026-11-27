// Package capital contains "api" as a substring and nothing to do with an API.
package capital

import "context"

// Recompute is a scheduled job with no caller context to inherit.
func Recompute() error {
	return rebuild(context.Background())
}

func rebuild(ctx context.Context) error {
	return ctx.Err()
}
