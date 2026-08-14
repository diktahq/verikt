package checker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/config"
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

	applyExcludes(result, []string{"**/testdata/**", "generated/**"})

	assert.Equal(t, []Violation{{File: "internal/cli/check.go", Message: "kept"}}, result.DependencyViolations)
	assert.Empty(t, result.FunctionViolations)
	assert.Empty(t, result.NamingViolations)
	assert.Len(t, result.StructureViolations, 1, "paths outside the globs are untouched")
	assert.Len(t, result.AntiPatternViolations, 1)
	assert.Equal(t, "internal/repo/db.go", result.AntiPatternViolations[0].File)
}

// The tests above call applyExcludes directly, which proves the function works
// and nothing about whether anything calls it. Commenting out both call sites
// left the package green. This one goes through CheckWithEngine, so it fails if
// the wiring is removed.
func TestCheckWithEngineAppliesExcludesToEngineFindings(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/app\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "gen"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gen", "api.go"),
		[]byte("package gen\n"), 0o644))

	cfg := &config.VeriktConfig{
		Language:     "go",
		Architecture: "hexagonal",
		Components:   []config.Component{{Name: "gen", In: []string{"gen"}}},
		Check:        config.CheckConfig{Exclude: []string{"gen/**"}},
	}

	result, err := CheckWithEngine(cfg, dir,
		antiPatternClientReturning([]AntiPattern{
			{File: "gen/api.go", Name: "sql_concatenation", Severity: "error"},
			{File: "internal/repo/db.go", Name: "sql_concatenation", Severity: "error"},
		}),
		dependencyClientReturning(nil),
		metricClientReturning([]Violation{{File: "gen/api.go", Message: "too long"}}),
	)

	require.NoError(t, err)
	require.Len(t, result.AntiPatternViolations, 1, "the excluded anti-pattern was not dropped")
	assert.Equal(t, "internal/repo/db.go", result.AntiPatternViolations[0].File)
	assert.Empty(t, result.FunctionViolations, "excludes apply to function metrics too")
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

	applyExcludes(result, []string{"**/testdata/**"})

	assert.Equal(t, 1, result.TotalViolations())
	assert.Equal(t, 9, result.RulesPassing)
}

// orphan_package must carry a project-relative path in File.
//
// It held the full import path, so no path-scoped feature could ever match one:
// check.exclude, severity_overrides paths, --staged and --diff all compare File
// against project-relative globs, and an import path never matches one. A waiver
// for "internal/stray/**" silently did nothing. Nothing asserted this, so the
// matching all those features depend on was unguarded.
func TestOrphanPackageFileIsProjectRelative(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/app\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "stray"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "stray", "x.go"),
		[]byte("package stray\n"), 0o644))

	cfg := &config.VeriktConfig{
		Language:     "go",
		Architecture: "hexagonal",
		Components:   []config.Component{{Name: "domain", In: []string{"internal/domain/**"}}},
	}

	violations := detectOrphanPackagesFS(cfg, dir)

	var orphan *Violation
	for i := range violations {
		if violations[i].Rule == "orphan_package" {
			orphan = &violations[i]
			break
		}
	}
	require.NotNil(t, orphan, "expected an orphan_package finding for internal/stray")

	assert.Equal(t, "internal/stray", orphan.File,
		"File must be project-relative so path-scoped globs can match it")
	assert.Contains(t, orphan.Message, "example.com/app/internal/stray",
		"the import path stays in the message, where it is useful to a reader")

	// The property that matters: a path-scoped exclude actually reaches it.
	assert.True(t, isExcluded(orphan.File, []string{"internal/stray/**"}),
		"a project-relative File is what makes check.exclude and severity_overrides work")
}
