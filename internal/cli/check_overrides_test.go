package cli

import (
	"testing"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeCheckerResult(depViolations, funcViolations []checker.Violation, antiPatterns []checker.AntiPattern) *checker.CheckResult {
	r := &checker.CheckResult{
		RulesChecked:          10,
		DependencyViolations:  depViolations,
		FunctionViolations:    funcViolations,
		AntiPatternViolations: antiPatterns,
	}
	r.RecalculateMetrics()
	return r
}

// TestApplySeverityOverrides_NoOverrides_NoChange verifies that passing empty overrides
// leaves the result unchanged.
func TestApplySeverityOverrides_NoOverrides_NoChange(t *testing.T) {
	result := makeCheckerResult(
		[]checker.Violation{{Rule: "dep", File: "a.go", Severity: "error"}},
		nil, nil,
	)
	before := result.TotalViolations()

	applySeverityOverrides(result, nil, config.SeverityOverrides{})

	assert.Equal(t, before, result.TotalViolations())
}

// TestApplySeverityOverrides_IgnoreRemovesViolation verifies that a matching "ignore"
// override removes the violation entirely.
func TestApplySeverityOverrides_IgnoreRemovesViolation(t *testing.T) {
	result := makeCheckerResult(
		[]checker.Violation{
			{Rule: "max_lines", File: "generated/big.go", Severity: "warning"},
			{Rule: "max_lines", File: "internal/svc.go", Severity: "warning"},
		},
		nil, nil,
	)

	overrides := config.SeverityOverrides{
		"max_lines": []config.SeverityOverride{
			{Severity: "ignore", Reason: "Generated code", Paths: []string{"generated/**"}},
		},
	}

	applySeverityOverrides(result, nil, overrides)

	require.Len(t, result.DependencyViolations, 1)
	assert.Equal(t, "internal/svc.go", result.DependencyViolations[0].File)
}

// TestApplySeverityOverrides_DowngradesErrorToWarning verifies severity downgrade.
func TestApplySeverityOverrides_DowngradesErrorToWarning(t *testing.T) {
	result := makeCheckerResult(
		[]checker.Violation{
			{Rule: "dep", File: "adapter/legacy/repo.go", Severity: "error"},
		},
		nil, nil,
	)

	overrides := config.SeverityOverrides{
		"dep": []config.SeverityOverride{
			{Severity: "should", Reason: "Legacy adapter", Paths: []string{"adapter/legacy/**"}},
		},
	}

	applySeverityOverrides(result, nil, overrides)

	require.Len(t, result.DependencyViolations, 1)
	assert.Equal(t, "warning", result.DependencyViolations[0].Severity)
}

// TestApplySeverityOverrides_AntiPatternsNotFiltered verifies that anti-pattern violations
// are never removed regardless of overrides.
func TestApplySeverityOverrides_AntiPatternsNotFiltered(t *testing.T) {
	antiPatterns := []checker.AntiPattern{
		{Name: "naked_goroutine", File: "cmd/main.go", Severity: "error"},
	}
	result := makeCheckerResult(nil, nil, antiPatterns)

	overrides := config.SeverityOverrides{
		"naked_goroutine": []config.SeverityOverride{
			{Severity: "ignore", Reason: "intentional", Paths: []string{}},
		},
	}

	applySeverityOverrides(result, nil, overrides)

	assert.Len(t, result.AntiPatternViolations, 1, "anti-pattern violations must not be filtered")
}

// TestApplySeverityOverrides_ProxyRuleIgnored verifies that proxy rule violations are
// removed when the override resolves to "ignore".
func TestApplySeverityOverrides_ProxyRuleIgnored(t *testing.T) {
	ruleResult := &rules.RunResult{
		Violations: []rules.RuleViolation{
			{RuleID: "cap-sql-parameterized", File: "generated/query.go", Severity: "error"},
			{RuleID: "cap-sql-parameterized", File: "internal/store.go", Severity: "error"},
		},
	}

	overrides := config.SeverityOverrides{
		"cap-sql-parameterized": []config.SeverityOverride{
			{Severity: "ignore", Reason: "Generated queries", Paths: []string{"generated/**"}},
		},
	}

	applySeverityOverrides(nil, ruleResult, overrides)

	require.Len(t, ruleResult.Violations, 1)
	assert.Equal(t, "internal/store.go", ruleResult.Violations[0].File)
}

// TestApplySeverityOverrides_RecalculatesMetrics verifies that metrics are updated after
// violations are removed by overrides.
func TestApplySeverityOverrides_RecalculatesMetrics(t *testing.T) {
	result := makeCheckerResult(
		[]checker.Violation{
			{Rule: "max_lines", File: "generated/a.go", Severity: "warning"},
			{Rule: "max_lines", File: "generated/b.go", Severity: "warning"},
		},
		nil, nil,
	)
	before := result.RulesPassing

	overrides := config.SeverityOverrides{
		"max_lines": []config.SeverityOverride{
			{Severity: "ignore", Reason: "Generated", Paths: []string{"generated/**"}},
		},
	}

	applySeverityOverrides(result, nil, overrides)

	assert.Greater(t, result.RulesPassing, before, "RulesPassing should increase after ignoring violations")
}

// TestCheck_SeverityOverride_PathScoping verifies that a single override key can apply
// different severities to different path globs. The first matching glob wins.
func TestCheck_SeverityOverride_PathScoping(t *testing.T) {
	result := &checker.CheckResult{
		RulesChecked: 5,
		FunctionViolations: []checker.Violation{
			{Rule: "max_lines", File: "adapter/grpc/handler.go", Severity: "error"},
			{Rule: "max_lines", File: "adapter/http/handler.go", Severity: "error"},
		},
	}
	result.RecalculateMetrics()

	overrides := config.SeverityOverrides{
		"max_lines": []config.SeverityOverride{
			{Severity: "ignore", Reason: "gRPC generated stubs", Paths: []string{"adapter/grpc/**"}},
			// No entry for adapter/http/** — default severity applies.
		},
	}

	applySeverityOverrides(result, nil, overrides)

	// gRPC violation removed; HTTP violation kept at original severity.
	require.Len(t, result.FunctionViolations, 1)
	assert.Equal(t, "adapter/http/handler.go", result.FunctionViolations[0].File)
	assert.Equal(t, "error", result.FunctionViolations[0].Severity)
}

// TestCheck_SeverityOverride_WithDiff verifies that --diff file filtering and severity
// overrides compose correctly: diff filters by file, overrides filter by severity.
func TestCheck_SeverityOverride_WithDiff(t *testing.T) {
	result := &checker.CheckResult{
		RulesChecked: 5,
		DependencyViolations: []checker.Violation{
			// In diff scope, should be overridden to ignore.
			{Rule: "arch/domain", File: "adapter/legacy/repo.go", Severity: "error"},
			// In diff scope, no override — kept.
			{Rule: "arch/domain", File: "adapter/new/repo.go", Severity: "error"},
			// NOT in diff scope — filtered out by diff, never reaches override.
			{Rule: "arch/domain", File: "service/order.go", Severity: "error"},
		},
	}
	result.RecalculateMetrics()

	// Step 1: apply --diff file filter (simulates filterCheckerResultByFiles).
	diffFiles := []string{"adapter/legacy/repo.go", "adapter/new/repo.go"}
	diffResult := filterCheckerResultByFiles(result, diffFiles)

	// Step 2: apply severity overrides.
	overrides := config.SeverityOverrides{
		"arch/domain": []config.SeverityOverride{
			{Severity: "ignore", Reason: "Legacy adapter, migrating", Paths: []string{"adapter/legacy/**"}},
		},
	}
	applySeverityOverrides(diffResult, nil, overrides)

	// Only adapter/new/repo.go should remain: legacy is overridden to ignore,
	// service/order.go was excluded by diff.
	require.Len(t, diffResult.DependencyViolations, 1)
	assert.Equal(t, "adapter/new/repo.go", diffResult.DependencyViolations[0].File)
}

// TestCheck_SeverityOverride_AntiPatternsNotOverridable verifies that a severity_override
// entry targeting an anti-pattern name has no effect — anti-patterns are safety rules.
func TestCheck_SeverityOverride_AntiPatternsNotOverridable(t *testing.T) {
	result := &checker.CheckResult{
		RulesChecked: 3,
		AntiPatternViolations: []checker.AntiPattern{
			{Name: "naked_goroutine", File: "cmd/worker.go", Severity: "error"},
		},
	}
	result.RecalculateMetrics()

	overrides := config.SeverityOverrides{
		"naked_goroutine": []config.SeverityOverride{
			{Severity: "ignore", Reason: "intentional", Paths: []string{}},
		},
	}

	applySeverityOverrides(result, nil, overrides)

	assert.Len(t, result.AntiPatternViolations, 1, "anti-pattern must survive any severity override")
	assert.Equal(t, "naked_goroutine", result.AntiPatternViolations[0].Name)
}

// TestFilterCheckerResultByFiles_RecalculatesMetrics verifies that the --diff file filter
// also recalculates metrics after removing violations.
func TestFilterCheckerResultByFiles_RecalculatesMetrics(t *testing.T) {
	original := &checker.CheckResult{
		RulesChecked: 5,
		DependencyViolations: []checker.Violation{
			{Rule: "dep", File: "a.go", Severity: "error"},
			{Rule: "dep", File: "b.go", Severity: "error"},
		},
	}
	original.RecalculateMetrics()

	filtered := filterCheckerResultByFiles(original, []string{"a.go"})

	assert.Len(t, filtered.DependencyViolations, 1)
	// RulesPassing should reflect the reduced violation count.
	assert.Equal(t, filtered.RulesChecked-filtered.TotalViolations(), filtered.RulesPassing)
}
