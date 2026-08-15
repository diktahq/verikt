package server

import "context"

// Start is a server lifecycle method: its goroutine is tied to the context and
// stops when the caller cancels.
func Start(ctx context.Context) {
	go func() {
		<-ctx.Done()
	}()
}
