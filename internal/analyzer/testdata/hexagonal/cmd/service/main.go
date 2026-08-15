package main

import (
	"fmt"
	"log/slog"

	httpadapter "example.com/hexagonal/internal/adapter/http"
	"example.com/hexagonal/internal/ginkgorunner"
)

func main() {
	_ = httpadapter.Handler()
	_ = ginkgorunner.Run()
	slog.Info("started")
	fmt.Errorf("wrapped: %w", fmt.Errorf("boom"))
}
