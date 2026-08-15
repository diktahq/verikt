// Package uuid stands in for github.com/google/uuid so the fixture needs no
// network access. The detector matches on the call text, not the resolved
// package.
package uuid

// New returns a v4-shaped identifier.
func New() string { return "" }
