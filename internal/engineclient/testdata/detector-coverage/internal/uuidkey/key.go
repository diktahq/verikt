package uuidkey

import "example.com/coverage/internal/uuid"

// Record uses a random UUID as its primary key.
type Record struct {
	ID string
}

func NewRecord() Record {
	return Record{ID: uuid.New()}
}
