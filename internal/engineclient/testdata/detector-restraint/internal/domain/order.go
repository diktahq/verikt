// Package domain depends on nothing outside itself.
package domain

// Order is a domain entity.
type Order struct {
	ID    string
	Total int64
}

// Valid reports whether the order can be placed.
func (o Order) Valid() bool { return o.ID != "" && o.Total > 0 }
