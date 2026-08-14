package checker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// check.exclude is documented as globs the checker ignores "entirely", but it was
// only wired into orphan-package detection. Anti-patterns and function metrics
// were reported for excluded paths regardless, so `verikt check` on this
// repository failed on its own test fixtures — files that exist precisely to
// trigger detectors, and that the Go toolchain excludes from builds.
func TestApplyExcludes(t *testing.T) {
	result := &CheckResult{
		DependencyViolations: []Violation{
			{File: "internal/cli/check.go", Message: "kept"},
			{File: "internal/checker/testdata/fixture/bad.go", Message: "dropped"},
		},
		FunctionViolations: []Violation{
			{File: "internal/checker/testdata/fixture/long.go", Message: "dropped"},
		},
		NamingViolations: []Violation{
			{File: "generated/api.pb.go", Message: "dropped"},
		},
		StructureViolations: []Violation{
			{File: "internal/cli/add.go", Message: "kept"},
		},
		AntiPatternViolations: []AntiPattern{
			{File: "internal/checker/testdata/fixture/repo.go", Name: "sql_concatenation"},
			{File: "internal/repo/db.go", Name: "sql_concatenation"},
		},
	}

	applyExcludes(result, []string{"testdata/**", "generated/**"})

	assert.Equal(t, []Violation{{File: "internal/cli/check.go", Message: "kept"}}, result.DependencyViolations)
	assert.Empty(t, result.FunctionViolations)
	assert.Empty(t, result.NamingViolations)
	assert.Len(t, result.StructureViolations, 1, "paths outside the globs are untouched")
	assert.Len(t, result.AntiPatternViolations, 1)
	assert.Equal(t, "internal/repo/db.go", result.AntiPatternViolations[0].File)
}

// With no globs configured nothing is dropped.
func TestApplyExcludes_NoGlobs(t *testing.T) {
	result := &CheckResult{
		AntiPatternViolations: []AntiPattern{{File: "internal/repo/db.go", Name: "sql_concatenation"}},
	}

	applyExcludes(result, nil)

	assert.Len(t, result.AntiPatternViolations, 1)
}

// Excluding findings must keep the reported metrics consistent.
func TestApplyExcludes_RecalculatesMetrics(t *testing.T) {
	result := &CheckResult{
		RulesChecked:         10,
		FunctionViolations:   []Violation{{File: "x/testdata/a.go"}},
		DependencyViolations: []Violation{{File: "internal/a.go"}},
	}

	applyExcludes(result, []string{"testdata/**"})

	assert.Equal(t, 1, result.TotalViolations())
	assert.Equal(t, 9, result.RulesPassing)
}
