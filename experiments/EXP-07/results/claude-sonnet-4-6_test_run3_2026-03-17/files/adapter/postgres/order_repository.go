```go
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/example/orders/domain"
	"github.com/example/orders/port"
)

// PostgresOrderRepository implements port.OrderRepository against a PostgreSQL database.
type PostgresOrderRepository struct {
	db *sql.DB
}

// NewPostgresOrderRepository constructs a repository backed by the given *sql.DB.
func NewPostgresOrderRepository(db *sql.DB) port.OrderRepository {
	return &PostgresOrderRepository{db: db}
}

// Save persists an order. Callers are responsible for providing a unique ID.
func (r *PostgresOrderRepository) Save(ctx context.Context, order *domain.Order) error {
	const q = `
		INSERT INTO orders (id, customer_id, total_cents, discount_code, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE
		SET status = EXCLUDED.status`

	_, err := r.db.ExecContext(ctx, q,
		order.ID,
		order.CustomerID,
		order.TotalCents,
		order.DiscountCode,
		string(order.Status),
		order.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres save order: %w", err)
	}
	return nil
}

// FindByID retrieves an order by its ID.
// Returns domain.ErrOrderNotFound when no row matches.
func (r *PostgresOrderRepository) FindByID(ctx context.Context, id string) (*domain.Order, error) {
	const q = `
		SELECT id, customer_id, total_cents, discount_code, status, created_at
		FROM orders
		WHERE id = $1`

	row := r.db.QueryRowContext(ctx, q, id)

	var o domain.Order
	var status string
	err := row.Scan(&o.ID, &o.CustomerID, &o.TotalCents, &o.DiscountCode, &status, &o.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrOrderNotFound
		}
		return nil, fmt.Errorf("postgres find order: %w", err)
	}

	o.Status = domain.OrderStatus(status)
	return &o, nil
}
```