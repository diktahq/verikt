// Package mcpadapter holds CLI commands. Its path contains "adapter", but a
// command run from a terminal has no request context to propagate.
package mcpadapter

import "context"

// RunServe is a CLI command: it starts the root context for its own run.
func RunServe(args []string) error {
	ctx := context.Background()
	return serve(ctx, args)
}

func serve(ctx context.Context, args []string) error {
	_ = args
	return ctx.Err()
}
