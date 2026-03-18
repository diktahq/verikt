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

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	orderRepo := postgres.NewPostgresOrderRepository(db)
	discRepo := postgres.NewPostgresDiscountRepository(db)
	svc := service.NewOrderService(orderRepo, discRepo)
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

**`domain/discount.go`** (new) — `Discount` entity with `Code`, `Percentage`, `Validate()`, and `Apply()`. `Apply` does integer arithmetic: `total - (total * pct / 100)`. Two sentinel errors: `ErrDiscountNotFound` and `ErrInvalidDiscount`.

**`domain/order.go`** — Added `DiscountCode string` to `Order`. Stored as the applied code (empty if no discount was used). The `TotalCents` on the order is already the post-discount price.

**`port/discount_repository.go`** (new) — `DiscountRepository` interface with a single `FindByCode` method.

**`port/order_service.go`** — `CreateOrderInput` gains optional `DiscountCode string`.

**`service/order_service.go`** — `NewOrderService` now takes a `DiscountRepository` (nil-safe, so tests without discounts don't need a stub). On creation: if a code is provided, look it up, apply it, and record the code on the order. An unknown code is mapped to `ErrInvalidOrder` so the HTTP layer returns 400.

**`adapter/postgres/discount_repository.go`** (new) — Reads from a `discounts(code, percentage)` table.

**`adapter/postgres/order_repository.go`** — `Save` and `FindByID` updated to include `discount_code` in the SQL. Schema migration needed: `ALTER TABLE orders ADD COLUMN discount_code TEXT NOT NULL DEFAULT '';` and `CREATE TABLE discounts (code TEXT PRIMARY KEY, percentage INT NOT NULL);`.

**`adapter/http/handler.go`** — `createOrderRequest` gains `discount_code`, `orderResponse` exposes it (omitted when empty).

**`cmd/orders/main.go`** — Wires the new `discRepo` into `NewOrderService`.