package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	adapthttp "github.com/example/orders/adapter/http"
	"github.com/example/orders/adapter/postgres"
	"github.com/example/orders/service"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL must be set")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	repo := postgres.NewPostgresOrderRepository(db)
	discRepo := postgres.NewPostgresDiscountRepository(db)
	svc := service.NewOrderService(repo, discRepo)
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