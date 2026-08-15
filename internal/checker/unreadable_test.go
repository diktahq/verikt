package checker

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/config"
)

// requireUnprivileged skips when the test can read anything regardless of mode.
func requireUnprivileged(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 000 does not prevent reading")
	}
}

// A directory the checker cannot read must be reported, not silently skipped —
// and it must not discard the findings from the rest of the tree.
//
// The walk returned the permission error, which aborted it, and the caller then
// did `if err != nil { return nil }` — throwing away every violation found so
// far. One unreadable directory anywhere under the project root turned orphan
// detection off completely and reported zero violations: a clean report for a
// tree that was never analysed.
func TestUnreadableDirectoryIsReportedAndDoesNotDiscardFindings(t *testing.T) {
	requireUnprivileged(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/app\n"), 0o644))

	// A package that is a genuine orphan, and must still be reported.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "stray"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "stray", "x.go"),
		[]byte("package stray\n"), 0o644))

	// A directory that cannot be read.
	locked := filepath.Join(dir, "internal", "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(locked, "y.go"), []byte("package locked\n"), 0o644))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	cfg := &config.VeriktConfig{
		Language:     "go",
		Architecture: "hexagonal",
		Components:   []config.Component{{Name: "domain", In: []string{"internal/domain/**"}}},
	}

	// The orphan walk must not abort or discard: it keeps finding orphans past
	// the unreadable directory.
	orphanViolations := detectOrphanPackagesFS(cfg, dir)

	var orphans []Violation
	for _, v := range orphanViolations {
		if v.Rule == "orphan_package" {
			orphans = append(orphans, v)
		}
	}
	assert.NotEmpty(t, orphans,
		"an unreadable directory must not discard findings from the rest of the tree")

	// And the unreadable path itself is reported by the dedicated walk, which
	// owns this and runs for every language.
	unreadable := detectUnreadablePaths(dir, nil)

	require.Len(t, unreadable, 1, "the unreadable directory must be reported")
	assert.Equal(t, "unreadable_path", unreadable[0].Rule)
	assert.Equal(t, "error", unreadable[0].Severity,
		"a path the checker could not read has not been checked, so it cannot pass")
	assert.Contains(t, unreadable[0].File, "locked")
	assert.Contains(t, unreadable[0].Message, "could not be read")
}

// A fully readable tree reports no unreadable paths.
func TestReadableTreeReportsNoUnreadablePaths(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/app\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "domain"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "domain", "x.go"),
		[]byte("package domain\n"), 0o644))

	cfg := &config.VeriktConfig{
		Language:     "go",
		Architecture: "hexagonal",
		Components:   []config.Component{{Name: "domain", In: []string{"internal/domain/**"}}},
	}

	assert.Empty(t, detectUnreadablePaths(dir, nil), "nothing here is unreadable")
	for _, v := range detectOrphanPackagesFS(cfg, dir) {
		assert.NotEqual(t, "unreadable_path", v.Rule, "nothing is unreadable here: %+v", v)
	}
}

// TypeScript projects must report unreadable paths too.
//
// The detection lives in the orphan-package walk, which is Go-only, so a
// TypeScript project with an unreadable directory got a clean report — the
// engine's own walkers discard read errors silently, and nothing else looked.
func TestUnreadablePathIsReportedForTypeScript(t *testing.T) {
	requireUnprivileged(t)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "domain"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "domain", "user.ts"),
		[]byte("export const a = 1;\n"), 0o644))

	locked := filepath.Join(dir, "src", "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	cfg := &config.VeriktConfig{
		Language:     "typescript",
		Architecture: "hexagonal",
		Components:   []config.Component{{Name: "domain", In: []string{"src/domain"}}},
	}

	result, err := CheckWithEngine(cfg, dir,
		emptyAntiPatternClient, dependencyClientReturning(nil), emptyMetricClient)
	require.NoError(t, err)

	var found bool
	for _, v := range result.DependencyViolations {
		if v.Rule == "unreadable_path" && strings.Contains(v.File, "locked") {
			found = true
			assert.Equal(t, "error", v.Severity)
		}
	}
	assert.True(t, found, "an unreadable directory must be reported for TypeScript too: %+v", result.DependencyViolations)
}

// A project with no go.mod must still report unreadable paths. The Go detection
// returns early without a module path, so it was the only thing looking.
func TestUnreadablePathIsReportedWithoutAModulePath(t *testing.T) {
	requireUnprivileged(t)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "domain"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "domain", "x.go"),
		[]byte("package domain\n"), 0o644))

	locked := filepath.Join(dir, "internal", "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	// No go.mod on purpose.
	violations := detectUnreadablePaths(dir, nil)

	require.Len(t, violations, 1)
	assert.Equal(t, "unreadable_path", violations[0].Rule)
	assert.Contains(t, violations[0].File, "locked")
}

// A source file that exists, is listed in its directory, but cannot be read is
// the residual case the directory walk does not see.
//
// WalkDir succeeds on it — Lstat works — so no error surfaces; only the read
// fails, and the read happens inside the Rust engine, which discards the error.
// The file is silently not analysed. Checking readability here closes it without
// a wire-format change: the walk already visits every path.
func TestUnreadableSourceFileIsReported(t *testing.T) {
	requireUnprivileged(t)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "domain"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "domain", "readable.go"),
		[]byte("package domain\n"), 0o644))

	locked := filepath.Join(dir, "internal", "domain", "locked.go")
	require.NoError(t, os.WriteFile(locked, []byte("package domain\n"), 0o644))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })

	violations := detectUnreadablePaths(dir, nil)

	require.Len(t, violations, 1, "the unreadable source file must be reported: %+v", violations)
	assert.Equal(t, "unreadable_path", violations[0].Rule)
	assert.Equal(t, "error", violations[0].Severity)
	assert.Contains(t, violations[0].File, "locked.go")
}

// Only files the analysis would actually read are checked. An unreadable binary
// asset is not a gap in the analysis, and opening every file in the tree to find
// out would be wasteful.
func TestUnreadableNonSourceFileIsIgnored(t *testing.T) {
	requireUnprivileged(t)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
	blob := filepath.Join(dir, "assets", "image.png")
	require.NoError(t, os.WriteFile(blob, []byte("\x89PNG"), 0o644))
	require.NoError(t, os.Chmod(blob, 0o000))
	t.Cleanup(func() { _ = os.Chmod(blob, 0o644) })

	assert.Empty(t, detectUnreadablePaths(dir, nil),
		"a non-source file the analysis never reads is not a gap")
}

// A readable tree with source files reports nothing.
func TestReadableSourceFilesReportNothing(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	for _, name := range []string{"a.go", "b.ts", "c.tsx", "d.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src", name), []byte("x\n"), 0o644))
	}

	assert.Empty(t, detectUnreadablePaths(dir, nil))
}

// A directory with its own go.mod is a different module and must not be
// analysed as part of this one.
//
// 0.2.0 stopped verikt.yaml being inherited across a module boundary, but the
// analysis still walked straight through it: a nested module was reported as an
// orphan package, at error severity, under an import path derived from the
// *parent* module — a path that does not exist. Found by dogfooding on a repo
// with tooling modules vendored inside it, where it failed the build.
func TestNestedModulesAreNotAnalysed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/parent\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "app", "app.go"),
		[]byte("package app\n"), 0o644))

	nested := filepath.Join(dir, "tools", "gen")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "go.mod"),
		[]byte("module example.com/unrelated\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "main.go"),
		[]byte("package gen\n"), 0o644))

	cfg := &config.VeriktConfig{
		Language:     "go",
		Architecture: "layered",
		Components:   []config.Component{{Name: "app", In: []string{"internal/app/**"}}},
	}

	for _, v := range detectOrphanPackagesFS(cfg, dir) {
		assert.NotContains(t, v.File, "tools/gen",
			"a nested module belongs to example.com/unrelated, not this project: %+v", v)
	}
}

// The boundary must not swallow the project it protects.
func TestParentModulePackagesAreStillAnalysed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/parent\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "stray"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "stray", "x.go"),
		[]byte("package stray\n"), 0o644))

	cfg := &config.VeriktConfig{
		Language:     "go",
		Architecture: "layered",
		Components:   []config.Component{{Name: "app", In: []string{"internal/app/**"}}},
	}

	var found bool
	for _, v := range detectOrphanPackagesFS(cfg, dir) {
		if v.Rule == "orphan_package" && v.File == "internal/stray" {
			found = true
		}
	}
	assert.True(t, found, "the parent module's own unclaimed package is still an orphan")
}
