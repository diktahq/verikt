```go
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

	// In production: import a driver package (e.g. pgx) for its side-effect init.
	// This stub uses database/sql directly; driver registration is left to the deployer.
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	repo := postgres.NewPostgresOrderRepository(db)
	discounts := postgres.NewPostgresDiscountRepository(db)
	svc := service.NewOrderService(repo, discounts)
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
```

---

What changed and why:

**`domain/discount.go`** — new file. `Discount` aggregate with `Code` and `Percentage`.

**`domain/order.go`** — added `DiscountCode` and `DiscountPercent` fields so the applied discount is stored with the order (auditable, no need to re-fetch).

**`port/discount_repository.go`** — new file. `FindByCode` is the only operation needed.

**`port/order_service.go`** — `DiscountCode` added to `CreateOrderInput`; empty string means no discount.

**`service/order_service.go`** — `NewOrderService` now takes a `DiscountRepository` (nil-safe). `applyDiscount` looks up the code, sets the fields, and reduces `TotalCents` before validation runs — so a discount that pushes the total to zero will still fail the `> 0` invariant.

**`adapter/http/handler.go`** — `discount_code` accepted in the request body; `discount_code` and `discount_percent` returned in the response (omitted when zero).

**`adapter/postgres/order_repository.go`** — updated SQL to persist/scan the two new columns.

**`adapter/postgres/discount_repository.go`** — new file. Expects a `discounts(code, percentage)` table.

**`cmd/orders/main.go`** — wires `PostgresDiscountRepository` into `NewOrderService`.