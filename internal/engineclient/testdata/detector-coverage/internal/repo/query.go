package repo

import "database/sql"

func Find(db *sql.DB, id string) (*sql.Rows, error) {
	query := "SELECT * FROM users WHERE id = '" + id + "'"
	return db.Query(query)
}
