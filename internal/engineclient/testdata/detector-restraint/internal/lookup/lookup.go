// Package lookup holds tables Go cannot express as constants.
package lookup

// Go has no const map or const slice, so a package-level var is the only way to
// write these. Nothing mutates them.
var categories = map[string]string{"http-api": "transport", "postgres": "data"}

var order = []string{"transport", "data"}

// Category returns the category for a capability.
func Category(name string) string { return categories[name] }

// Order returns the display order.
func Order() []string { return order }
