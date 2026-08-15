package store

import "database/sql"

// Find uses a parameterised query.
func Find(db *sql.DB, id string) (*sql.Rows, error) {
	return db.Query("SELECT * FROM users WHERE id = ?", id)
}
