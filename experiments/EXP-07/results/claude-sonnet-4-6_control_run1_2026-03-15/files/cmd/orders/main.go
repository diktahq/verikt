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
	discountRepo := postgres.NewPostgresDiscountRepository(db)
	svc := service.NewOrderService(repo, discountRepo)
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

A few notes on the design choices:

**Discount applied before validation.** `ApplyDiscount` runs before `Validate`, so the post-discount `TotalCents` is what gets validated and persisted — no separate "original total" field needed unless you want it.

**Unknown code is a 400.** An unrecognised `discount_code` wraps `ErrInvalidOrder`, so the handler returns a `400 Bad Request` rather than a 500. That's the right signal — the client sent bad input.

**Postgres schema** you'll need for discounts:
```sql
CREATE TABLE discounts (
    code        TEXT PRIMARY KEY,
    percentage  INT NOT NULL CHECK (percentage BETWEEN 1 AND 100)
);
```

And add `discount_code`, `discount_pct` columns to `orders`:
```sql
ALTER TABLE orders ADD COLUMN discount_code TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN discount_pct  INT  NOT NULL DEFAULT 0;
```