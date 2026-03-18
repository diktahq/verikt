package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	adapthttp "github.com/example/orders/adapter/http"
	"github.com/example/orders/adapter/memory"
	"github.com/example/orders/adapter/postgres"
	"github.com/example/orders/domain"
	"github.com/example/orders/service"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL must be set")
	}

	// In production: import a driver package (e.g. pgx) for its side-effect init.
	// This stub uses database/sql directly; driver registration is left to the deployer.
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	discRepo := memory.NewDiscountRepository([]domain.Discount{
		{Code: "SAVE10", Percent: 10},
		{Code: "SAVE20", Percent: 20},
	})

	repo := postgres.NewPostgresOrderRepository(db)
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