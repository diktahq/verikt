// Command api is a service entry point in a package whose path contains "api".
// main has no request in scope, so context.Background() here drops nothing:
// it is the root every request context is later derived from.
package main

import (
	"context"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	otelShutdown := setupTelemetry()
	defer otelShutdown(context.Background())

	// A background worker started from main has no request to inherit from.
	go refresh(context.Background())

	<-ctx.Done()
}

func setupTelemetry() func(context.Context) {
	return func(context.Context) {}
}

func refresh(ctx context.Context) {
	<-ctx.Done()
}
