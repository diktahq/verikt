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

Here's what changed and why each decision was made:

**`domain/discount.go`** — New type with `Code` and `Percentage`. `Apply` lives on the discount itself; the discount knows how to apply itself to an amount.

**`domain/order.go`** — Added `DiscountCode string` to record which code was used. The stored `TotalCents` is always the post-discount final amount — no need for a separate field.

**`port/discount_repository.go`** — Single-method interface: `FindByCode`. Keeps it minimal.

**`port/order_service.go`** — `DiscountCode string` added to `CreateOrderInput`, empty string means no discount.

**`service/order_service.go`** — `NewOrderService` now takes a `DiscountRepository`. Discount resolution is extracted into `applyDiscount` to keep `CreateOrder` under the 50-line rule. An invalid code is a 422-class error, not a 500.

**`adapter/postgres/discount_repository.go`** — Assumes a `discounts(code, percentage)` table. Straightforward scan, maps `sql.ErrNoRows` to `domain.ErrDiscountNotFound`.

**`adapter/postgres/order_repository.go`** — `discount_code` column added to both `INSERT` and `SELECT`.

**`adapter/http/handler.go`** — `discount_code` in request and response (omitted when empty). Invalid discount code returns `422 Unprocessable Entity` — it's a valid request with an unresolvable reference, not a malformed request.