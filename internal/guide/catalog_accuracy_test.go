package guide

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
	// The per-provider loop used to read
	//   if !available[name] { continue }; require.Contains(t, available, name)
	// which only reached the assertion once it was already true. Both subtests
	// asserted nothing at all.
	all := map[string]bool{}
	for _, provider := range []string{"golang", "typescript"} {
		for name := range providerCapabilities(t, provider) {
			all[name] = true
		}
	}
	require.NotEmpty(t, all, "no capabilities discovered — the test cannot check anything")

	for name := range whenToUse {
		require.Contains(t, all, name,
			"whenToUse describes %q, which no provider ships — stale entries put wrong information in the generated guide", name)
	}
}

// Every shipped capability needs guidance, or the generated catalog has a blank
// where an agent expects a reason to choose it.
func TestEveryCapabilityHasWhenToUseGuidance(t *testing.T) {
	for _, provider := range []string{"golang", "typescript"} {
		for name := range providerCapabilities(t, provider) {
			require.Contains(t, whenToUse, name,
				"%s ships capability %q with no whenToUse entry", provider, name)
		}
	}
}

// The guidance must not claim a capability does something its files do not.
//
// This is what the header of the test above always claimed to guard and never
// did: it checked that keys named real capabilities, never that the text was
// true. `ci-github` was described to agents as a "GitHub Actions CI/CD pipeline"
// while it ships three markdown templates and its own capability.yaml says so —
// a guide that is confidently wrong is worse than one that is silent.
//
// Claiming a pipeline is mechanically checkable: a pipeline capability has to
// ship a CI configuration file, not documentation.
func TestPipelineGuidanceRequiresAPipelineFile(t *testing.T) {
	pipelineClaim := regexp.MustCompile(`(?i)\b(ci/cd|pipeline)\b`)

	for _, provider := range []string{"golang", "typescript"} {
		for name := range providerCapabilities(t, provider) {
			guidance, ok := whenToUse[name]
			if !ok || !pipelineClaim.MatchString(guidance) {
				continue
			}

			files := capabilityFiles(t, provider, name)
			require.NotEmpty(t, files, "%s/%s ships no files at all", provider, name)

			hasConfig := false
			for _, f := range files {
				if strings.HasSuffix(f, ".yml.tmpl") || strings.HasSuffix(f, ".yaml.tmpl") {
					hasConfig = true
					break
				}
			}
			require.True(t, hasConfig,
				"whenToUse tells agents %q is %q, but it ships only %v — no CI configuration",
				name, guidance, files)
		}
	}
}

// The description an agent reads must match the capability's own declaration.
func TestGuidanceDoesNotContradictCapabilityDescription(t *testing.T) {
	// ci-github is the case that drifted; pin it against its capability.yaml so
	// the two cannot diverge again silently.
	description := capabilityDescription(t, "golang", "ci-github")
	require.Contains(t, strings.ToLower(description), "template",
		"capability.yaml changed; update this guard deliberately")
	require.Contains(t, strings.ToLower(whenToUse["ci-github"]), "template",
		"whenToUse contradicts ci-github's own description (%q)", description)
	require.NotRegexp(t, `(?i)\b(ci/cd|actions|pipeline)\b`, whenToUse["ci-github"],
		"ci-github ships issue and PR templates, not a pipeline")
}

// capabilityFiles returns the template file paths a capability ships.
func capabilityFiles(t *testing.T, provider, name string) []string {
	t.Helper()
	root := filepath.Join("..", "..", "providers", provider, "templates", "capabilities", name, "files")
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			out = append(out, filepath.Base(path))
		}
		return nil
	})
	require.NoError(t, err)
	return out
}

// capabilityDescription reads a capability's declared description.
func capabilityDescription(t *testing.T, provider, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "providers", provider, "templates", "capabilities", name, "capability.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var manifest struct {
		Description string `yaml:"description"`
	}
	require.NoError(t, yaml.Unmarshal(data, &manifest))
	require.NotEmpty(t, manifest.Description, "%s has no description", path)
	return manifest.Description
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
