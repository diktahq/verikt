package checker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/verikt/internal/config"
	"github.com/stretchr/testify/require"
)

// projectWithPackages writes a module containing one Go file per named package
// directory, and returns the project root.
func projectWithPackages(t *testing.T, dirs ...string) string {
	t.Helper()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"),
		[]byte("module example.com/app\n\ngo 1.24\n"), 0o644))

	for _, dir := range dirs {
		full := filepath.Join(root, filepath.FromSlash(dir))
		require.NoError(t, os.MkdirAll(full, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(full, "code.go"),
			[]byte("package "+filepath.Base(dir)+"\n"), 0o644))
	}
	return root
}

// Coverage was decided by os.Stat on the pattern with "/**" trimmed off — a
// directory-existence test, not a glob match. `in: ["**"]` became a stat for a
// directory literally named "**", which no project has, so a catch-all
// component was reported as missing_component at *error* severity. It failed
// the build on four repositories in a corpus audit, and the only remedy
// available to the user was to write a less expressive config.
//
// Every pattern here is one the documented glob semantics accept.
func TestDeclaredComponentsAreMatchedByGlobNotByDirectoryExistence(t *testing.T) {
	root := projectWithPackages(t, "internal/domain", "internal/adapter/postgres", "cmd/api")

	for _, pattern := range []string{
		"**",                  // catch-all: the case that failed
		"internal/**",         // the shape that happened to work
		"internal/adapter/",   // trailing slash means a directory
		"internal",            // bare name covers the tree below it
		"**/postgres/**",      // at any depth
		"internal/*/postgres", // single-segment wildcard
	} {
		t.Run(pattern, func(t *testing.T) {
			cfg := &config.VeriktConfig{
				Language:   "go",
				Components: []config.Component{{Name: "claimed", In: []string{pattern}}},
			}

			require.Empty(t, detectMissingComponents(cfg, root),
				"pattern %q matches source directories, so the component is not missing", pattern)
			require.Equal(t, 1, countCoveredComponents(cfg, root))
		})
	}
}

// A component whose pattern genuinely matches nothing is still reported — the
// fix must not turn the detector off.
func TestComponentMatchingNoSourceDirectoryIsReported(t *testing.T) {
	root := projectWithPackages(t, "internal/domain")

	cfg := &config.VeriktConfig{
		Language:   "go",
		Components: []config.Component{{Name: "absent", In: []string{"internal/transport/**"}}},
	}

	violations := detectMissingComponents(cfg, root)
	require.Len(t, violations, 1)
	require.Equal(t, "missing_component", violations[0].Rule)
	require.Equal(t, 0, countCoveredComponents(cfg, root))
}

// The defect was not that one matcher was wrong, but that two disagreed: the
// same catch-all component claimed every package for orphan detection while
// coverage saw it claim nothing. Both now use graph.MatchesComponent, so the
// contradiction is unrepresentable — pinned here because it is the property
// that matters, not either detector's answer in isolation.
func TestCoverageAndOrphanDetectionAgree(t *testing.T) {
	root := projectWithPackages(t, "internal/domain", "internal/adapter/postgres", "cmd/api")

	for _, pattern := range []string{"**", "internal/**", "cmd/**"} {
		t.Run(pattern, func(t *testing.T) {
			cfg := &config.VeriktConfig{
				Language:     "go",
				Architecture: "layered",
				Components:   []config.Component{{Name: "claimed", In: []string{pattern}}},
			}

			claimsSomething := countCoveredComponents(cfg, root) > 0
			orphans := detectOrphanPackagesFS(cfg, root)
			claimsEverything := len(orphans) == 0

			require.True(t, claimsSomething,
				"orphan detection reports %d unclaimed of 3 packages, so the component claims something", len(orphans))
			if claimsEverything {
				require.True(t, claimsSomething,
					"a component that leaves no orphans cannot also match no directory")
			}
		})
	}
}

// Test files are not source (INV-004): a directory holding only tests is not a
// directory the component covers.
func TestDirectoryHoldingOnlyTestFilesIsNotCoverage(t *testing.T) {
	root := projectWithPackages(t, "internal/domain")

	testOnly := filepath.Join(root, "internal", "transport")
	require.NoError(t, os.MkdirAll(testOnly, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(testOnly, "handler_test.go"),
		[]byte("package transport\n"), 0o644))

	cfg := &config.VeriktConfig{
		Language:   "go",
		Components: []config.Component{{Name: "transport", In: []string{"internal/transport/**"}}},
	}

	require.Len(t, detectMissingComponents(cfg, root), 1,
		"a directory containing only _test.go files holds no source the detectors see")
}
