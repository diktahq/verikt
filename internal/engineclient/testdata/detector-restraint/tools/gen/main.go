// Package gen belongs to example.com/unrelated-tooling, a separate module that
// happens to live inside this directory tree.
//
// Reduced from a real project where verikt walked through the module boundary
// and reported this package as an orphan, at error severity, under an import
// path derived from the parent module — a path that does not exist.
package gen

// Generate does nothing; the point is the go.mod beside it.
func Generate() string { return "generated" }
