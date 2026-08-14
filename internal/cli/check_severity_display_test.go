package cli

import (
	"testing"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/stretchr/testify/assert"
)

// Only error severity fails the check, so a warning must not carry the same
// marker as a failure. verikt check on this repository printed 51 warning-level
// findings as ✗ while exiting 0.
func TestPrintViolationSection_MarkerFollowsSeverity(t *testing.T) {
	out := captureStdout(t, func() {
		printViolationSection("FUNCTION VIOLATIONS", []checker.Violation{
			{Severity: "warning", File: "internal/cli/add.go", Line: 39, Message: "107 lines (max: 50)"},
			{Severity: "error", File: "domain/order.go", Line: 4, Message: "domain must not depend on adapters"},
		})
	})

	assert.Contains(t, out, "⚠ internal/cli/add.go:39", "warnings must not use the failure marker")
	assert.Contains(t, out, "✗ domain/order.go:4")
}
