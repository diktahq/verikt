package checker

import (
	"os"
	"path/filepath"
	"runtime"
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

	violations := detectOrphanPackagesFS(cfg, dir)

	var orphans, unreadable []Violation
	for _, v := range violations {
		switch v.Rule {
		case "orphan_package":
			orphans = append(orphans, v)
		case "unreadable_path":
			unreadable = append(unreadable, v)
		}
	}

	assert.NotEmpty(t, orphans,
		"an unreadable directory must not discard findings from the rest of the tree")

	require.Len(t, unreadable, 1, "the unreadable directory must be reported")
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

	for _, v := range detectOrphanPackagesFS(cfg, dir) {
		assert.NotEqual(t, "unreadable_path", v.Rule, "nothing is unreadable here: %+v", v)
	}
}
