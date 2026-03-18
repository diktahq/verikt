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
```

---

A few notes on what changed and why:

**`domain/discount.go`** — `Discount` owns the `Apply()` logic. The percentage reduction stays in the domain, not the service.

**`domain/order.go`** — `DiscountCode` and `DiscountPct` added so the applied discount is recorded on the order. This matters for auditing and retrieval.

**`service/order_service.go`** — `NewOrderService` now takes a `DiscountRepository`. If a code is provided and not found, the error surfaces as `ErrDiscountNotFound` — the caller decides the HTTP status, not the service.

**`adapter/http/handler.go`** — Invalid discount code returns `422 Unprocessable Entity`, not `400`. The request was well-formed; the code just doesn't exist.

**Database** — you'll need a migration:
```sql
CREATE TABLE discounts (
    code        TEXT PRIMARY KEY,
    percentage  INT NOT NULL CHECK (percentage BETWEEN 1 AND 100)
);

ALTER TABLE orders
    ADD COLUMN discount_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN discount_pct  INT  NOT NULL DEFAULT 0;
```