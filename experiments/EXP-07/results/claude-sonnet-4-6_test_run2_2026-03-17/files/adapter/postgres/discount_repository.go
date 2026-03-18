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

type PostgresDiscountRepository struct {
	db *sql.DB
}

func NewPostgresDiscountRepository(db *sql.DB) port.DiscountRepository {
	return &PostgresDiscountRepository{db: db}
}

func (r *PostgresDiscountRepository) FindByCode(ctx context.Context, code string) (*domain.Discount, error) {
	const q = `SELECT code, percentage FROM discounts WHERE code = $1`

	row := r.db.QueryRowContext(ctx, q, code)

	var d domain.Discount
	err := row.Scan(&d.Code, &d.Percentage)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDiscountNotFound
		}
		return nil, fmt.Errorf("postgres find discount: %w", err)
	}

	return &d, nil
}
```