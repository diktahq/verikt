package cli

import (
	"testing"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/rules"
	"github.com/stretchr/testify/assert"
)

// Only error severity fails the check, so a warning must not carry the same
// marker as a failure. verikt check on this repository printed 51 warning-level
// findings as ✗ while exiting 0.
//
// The branch was duplicated in three print functions and fixed in one. Each copy
// gets a test here, because the findings that produced the reported symptom were
// mostly warning-level anti-patterns — the copy that had no test.
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

func TestPrintAntiPatternSection_MarkerFollowsSeverity(t *testing.T) {
	out := captureStdout(t, func() {
		printAntiPatternSection("ANTI-PATTERN VIOLATIONS", []checker.AntiPattern{
			{Name: "god_package", Severity: "warning", File: "internal/core", Message: "72 exported symbols"},
			{Name: "sql_concatenation", Severity: "error", File: "repo/db.go", Line: 12, Message: "concatenated SQL"},
		})
	})

	assert.Contains(t, out, "⚠ [god_package] internal/core", "warning-level anti-patterns do not fail the check")
	assert.Contains(t, out, "✗ [sql_concatenation] repo/db.go:12")
}

func TestPrintProxyRuleSection_MarkerFollowsSeverity(t *testing.T) {
	out := captureStdout(t, func() {
		printProxyRuleSection(&rules.RunResult{
			Statuses: []rules.RuleStatus{
				{Rule: rules.Rule{ID: "warn-rule"}, Filename: "warn.yaml", Status: "valid"},
				{Rule: rules.Rule{ID: "err-rule"}, Filename: "err.yaml", Status: "valid"},
			},
			Violations: []rules.RuleViolation{
				{RuleID: "warn-rule", Severity: "warning", File: "internal/a.go", Line: 3, Description: "prefer slog"},
				{RuleID: "err-rule", Severity: "error", File: "internal/b.go", Line: 7, Description: "no panic"},
			},
		})
	})

	assert.Contains(t, out, "⚠ [warn-rule] internal/a.go:3")
	assert.Contains(t, out, "✗ [err-rule] internal/b.go:7")
}

// A stale or invalid rule did not run, and ruleResultBlocks exits 1 for it. It
// must therefore print as a failure: otherwise the build fails with no ✗ visible
// anywhere on screen, and the operator goes looking for a problem that is not
// where the output points.
func TestPrintProxyRuleSection_UnrunRulesUseFailureMarker(t *testing.T) {
	out := captureStdout(t, func() {
		printProxyRuleSection(&rules.RunResult{
			Statuses: []rules.RuleStatus{
				{Rule: rules.Rule{ID: "gone"}, Filename: "gone.yaml", Status: "stale", Error: "scope matches 0 files"},
				{Rule: rules.Rule{ID: "broken"}, Filename: "broken.yaml", Status: "invalid", Error: "bad regex"},
			},
		})
	})

	assert.Contains(t, out, "✗ [gone.yaml] stale:", "a stale rule fails the build, so it is not a warning")
	assert.Contains(t, out, "✗ [broken.yaml] invalid:")
	assert.NotContains(t, out, "⚠", "nothing here is advisory — every line shown fails the build")
}
