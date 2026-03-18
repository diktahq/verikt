package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	adapthttp "github.com/example/orders/adapter/http"
	"github.com/example/orders/adapter/payments"
	"github.com/example/orders/adapter/postgres"
	"github.com/example/orders/service"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL must be set")
	}

	paymentsBaseURL := os.Getenv("PAYMENTS_API_URL")
	if paymentsBaseURL == "" {
		log.Fatal("PAYMENTS_API_URL must be set")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	paymentsClient := payments.NewClient(paymentsBaseURL, &http.Client{
		Timeout: 10 * time.Second,
	})

	repo := postgres.NewPostgresOrderRepository(db)
	svc := service.NewOrderService(repo, paymentsClient)
	h := adapthttp.NewHandler(svc)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", h.CreateOrder)
	mux.HandleFunc("GET /orders/{id}", h.GetOrder)

	addr := ":8080"
	fmt.Printf("orders service listening on %s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}