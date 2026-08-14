package guide

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWhenToUseKeysAreRealCapabilities asserts the hardcoded whenToUse map does not
// describe capabilities that do not exist.
//
// The guide's catalog takes each capability's purpose from its capability.yaml but the
// "when to consider" column from this map, so the two can drift. They had: the map told
// agents ci-github was a "GitHub Actions CI/CD pipeline" while the capability ships issue
// and PR templates and its capability.yaml says so. A stale entry here is worse than a
// missing one — it is wrong information in the file agents read.
func TestWhenToUseKeysAreRealCapabilities(t *testing.T) {
	for _, provider := range []string{"golang", "typescript"} {
		t.Run(provider, func(t *testing.T) {
			// Only assert for capabilities this provider ships; the map spans both.
			available := providerCapabilities(t, provider)
			for name := range whenToUse {
				if !available[name] {
					continue
				}
				require.Contains(t, available, name)
			}
		})
	}

	// Every key must exist in at least one provider.
	all := map[string]bool{}
	for _, provider := range []string{"golang", "typescript"} {
		for name := range providerCapabilities(t, provider) {
			all[name] = true
		}
	}
	for name := range whenToUse {
		require.Contains(t, all, name,
			"whenToUse describes %q, which no provider ships — stale entries put wrong information in the generated guide", name)
	}
}

// providerCapabilities returns the capability names a provider ships.
func providerCapabilities(t *testing.T, provider string) map[string]bool {
	t.Helper()
	root := filepath.Join("..", "..", "providers", provider, "templates", "capabilities")
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	out := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			out[e.Name()] = true
		}
	}
	return out
}
