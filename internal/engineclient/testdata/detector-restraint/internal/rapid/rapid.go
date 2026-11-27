// Package rapid contains "api" as a substring and nothing to do with an API.
package rapid

import "context"

// Run starts a batch with no caller context to inherit.
func Run() error {
	return process(context.Background())
}

func process(ctx context.Context) error {
	return ctx.Err()
}
