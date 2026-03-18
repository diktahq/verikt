package checker

import (
	"testing"
)

func TestRecalculateMetrics_AfterFiltering(t *testing.T) {
	r := &CheckResult{
		RulesChecked: 10,
		DependencyViolations: []Violation{
			{Rule: "dep", File: "a.go", Severity: "error"},
			{Rule: "dep", File: "b.go", Severity: "error"},
		},
		FunctionViolations: []Violation{
			{Rule: "max_lines", File: "c.go", Severity: "warning"},
		},
	}

	r.RecalculateMetrics()

	if r.RulesPassing != 7 {
		t.Errorf("expected RulesPassing=7, got %d", r.RulesPassing)
	}

	// Simulate filtering: remove one violation.
	r.DependencyViolations = r.DependencyViolations[:1]
	r.RecalculateMetrics()

	if r.RulesPassing != 8 {
		t.Errorf("after filtering, expected RulesPassing=8, got %d", r.RulesPassing)
	}
}

func TestRecalculateMetrics_NeverGoesNegative(t *testing.T) {
	r := &CheckResult{
		RulesChecked: 1,
		DependencyViolations: []Violation{
			{Rule: "dep", File: "a.go"},
			{Rule: "dep", File: "b.go"},
			{Rule: "dep", File: "c.go"},
		},
	}
	r.RecalculateMetrics()
	if r.RulesPassing != 0 {
		t.Errorf("expected RulesPassing=0, got %d", r.RulesPassing)
	}
}
