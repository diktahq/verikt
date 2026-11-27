package guide

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/verikt/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSplitContent_CorrectFileCount(t *testing.T) {
	opts := GenerateOptions{
		Architecture: "hexagonal",
		Capabilities: []string{"http-api", "mysql", "auth-jwt"},
		Components: []config.Component{
			{Name: "domain", In: []string{"domain/**"}, MayDependOn: []string{}},
		},
	}

	sc := buildSplitContent(opts)

	assert.NotEmpty(t, sc.Index)
	// http-api -> http, mysql -> data, auth-jwt -> security
	assert.Len(t, sc.Categories, 3)
	assert.Contains(t, sc.Categories, "http")
	assert.Contains(t, sc.Categories, "data")
	assert.Contains(t, sc.Categories, "security")
}

func TestBuildIndexContent_CompactSummary(t *testing.T) {
	opts := GenerateOptions{
		Architecture: "hexagonal",
		Capabilities: []string{"http-api", "mysql"},
		Components: []config.Component{
			{Name: "domain", In: []string{"domain/**"}, MayDependOn: []string{}},
			{Name: "service", In: []string{"service/**"}, MayDependOn: []string{"domain"}},
		},
	}

	content := buildIndexContent(opts)

	// Claude Code loads a rule with no paths frontmatter at launch; that is
	// what the always-loaded index needs, so it carries no frontmatter at all.
	assert.True(t, strings.HasPrefix(content, claudeHeader), "index must start with the generated header, not frontmatter")
	assert.NotContains(t, content, "globs:")
	assert.Contains(t, content, "Architecture: hexagonal")
	assert.Contains(t, content, "Hexagonal (ports & adapters)")
	assert.Contains(t, content, "## Layer Rules")
	assert.Contains(t, content, "**domain**: depends on nothing (innermost)")
	assert.Contains(t, content, "## Active Capabilities")
	assert.Contains(t, content, "**http-api**")
	assert.Contains(t, content, "**mysql**")
	assert.Contains(t, content, "## Critical Warnings")

	// Should NOT contain full adding code or anti-patterns (those go in category files).
	assert.NotContains(t, content, "## Adding Code")
	assert.NotContains(t, content, "## Anti-patterns")
}

func TestBuildCategoryContent_CorrectGlobs(t *testing.T) {
	opts := GenerateOptions{
		Architecture: "hexagonal",
		Capabilities: []string{"http-api"},
	}

	content := buildCategoryContent("http", []string{"http-api"}, opts)

	assert.Contains(t, content, `"internal/handler/**"`)
	assert.Contains(t, content, `"adapter/httphandler/**"`)
	assert.Contains(t, content, "# verikt — HTTP & Transport")
	assert.Contains(t, content, "http-api")
	assert.Contains(t, content, "## Adding Code")
}

func TestInstalledInCategory(t *testing.T) {
	tests := []struct {
		name      string
		category  string
		installed []string
		want      []string
	}{
		{
			name:      "http caps filtered",
			category:  "http",
			installed: []string{"http-api", "mysql", "grpc"},
			want:      []string{"http-api", "grpc"},
		},
		{
			name:      "no match",
			category:  "frontend",
			installed: []string{"http-api", "mysql"},
			want:      nil,
		},
		{
			name:      "bff in http category",
			category:  "http",
			installed: []string{"bff", "mysql"},
			want:      []string{"bff"},
		},
		{
			name:      "unknown category",
			category:  "nonexistent",
			installed: []string{"http-api"},
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := installedInCategory(tt.category, tt.installed)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestSplitVsMonolithic_NonClaude(t *testing.T) {
	dir := t.TempDir()
	opts := GenerateOptions{
		ProjectDir:   dir,
		Target:       "cursor",
		Architecture: "hexagonal",
		Capabilities: []string{"http-api", "mysql"},
	}

	err := Generate(opts)
	require.NoError(t, err)

	// Cursor gets the compact writer rules; Adding Code is on demand.
	data, err := os.ReadFile(filepath.Join(dir, ".cursor", "rules", "verikt.mdc"))
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "Architecture: hexagonal")
	assert.NotContains(t, content, "## Adding Code")
	assert.Contains(t, content, ".verikt/references/architecture.md")

	ref, err := os.ReadFile(filepath.Join(dir, ".verikt", "references", "architecture.md"))
	require.NoError(t, err)
	assert.Contains(t, string(ref), "## Adding Code")
}

func TestSplitCleanup_RemovesOldFile(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".claude", "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o755))

	// Create old monolithic file, as every verikt since 0.1.0 wrote it.
	oldPath := filepath.Join(rulesDir, "verikt.md")
	require.NoError(t, os.WriteFile(oldPath, []byte(claudeHeader+"old content"), 0o644))

	opts := GenerateOptions{
		ProjectDir:   dir,
		Target:       "claude",
		Architecture: "hexagonal",
		Capabilities: []string{"http-api"},
	}

	err := Generate(opts)
	require.NoError(t, err)

	// Old file should be deleted.
	assert.NoFileExists(t, oldPath)
	// New split files should exist.
	assert.FileExists(t, filepath.Join(rulesDir, "verikt-index.md"))
	assert.FileExists(t, filepath.Join(rulesDir, "verikt-http.md"))
}

func TestCatalogOnly_StaysMonolithic(t *testing.T) {
	dir := t.TempDir()
	opts := GenerateOptions{
		ProjectDir:   dir,
		Target:       "claude",
		Architecture: "hexagonal",
		Capabilities: []string{"http-api"},
		CatalogOnly:  true,
		TemplateFS:   testCapFS(),
	}

	err := Generate(opts)
	require.NoError(t, err)

	// CatalogOnly uses monolithic output.
	assert.FileExists(t, filepath.Join(dir, ".claude", "rules", "verikt.md"))
	// No split files.
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "rules", "verikt-index.md"))
}

func TestGlobsForCategory(t *testing.T) {
	globs := globsForCategory("http")
	assert.Contains(t, globs, "internal/handler/**")
	assert.Contains(t, globs, "adapter/httphandler/**")

	// Unknown category returns wildcard.
	globs = globsForCategory("nonexistent")
	assert.Equal(t, []string{"**/*"}, globs)
}

func TestBuildSplitContent_OnlyInstalledCategories(t *testing.T) {
	opts := GenerateOptions{
		Architecture: "flat",
		Capabilities: []string{"testing"},
	}

	sc := buildSplitContent(opts)

	// Only quality category should be present.
	assert.Len(t, sc.Categories, 1)
	assert.Contains(t, sc.Categories, "quality")
}

func TestSplitFileTokenCompliance(t *testing.T) {
	dir := t.TempDir()
	opts := GenerateOptions{
		ProjectDir:   dir,
		Target:       "claude",
		Architecture: "hexagonal",
		Capabilities: []string{
			"http-api", "mysql", "redis", "auth-jwt", "cors",
			"rate-limiting", "health", "observability", "docker", "testing",
		},
		Components: []config.Component{
			{Name: "domain", In: []string{"domain/**"}, MayDependOn: []string{}},
			{Name: "service", In: []string{"service/**"}, MayDependOn: []string{"domain"}},
			{Name: "adapters", In: []string{"adapter/**"}, MayDependOn: []string{"domain"}},
		},
	}

	require.NoError(t, Generate(opts))

	entries, err := os.ReadDir(filepath.Join(dir, ".claude", "rules"))
	require.NoError(t, err)

	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "verikt-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, ".claude", "rules", e.Name()))
		require.NoError(t, err)
		words := len(strings.Fields(string(data)))
		approxTokens := int(float64(words) * 1.3)
		if approxTokens > 1500 {
			t.Errorf("file %s too large: ~%d tokens (%d words)", e.Name(), approxTokens, words)
		}
	}
}

func TestBuildIndexContent_WithFeatures(t *testing.T) {
	opts := GenerateOptions{
		Architecture:    "hexagonal",
		Capabilities:    []string{"http-api"},
		LanguageVersion: "Go 1.24",
		Features: map[string]bool{
			"slices_package":  true,
			"range_over_int":  true,
			"range_over_func": true,
			"os_root":         true,
			"weak_pointers":   false,
		},
		Components: []config.Component{
			{Name: "domain", In: []string{"domain/**"}, MayDependOn: []string{}},
		},
	}

	content := buildIndexContent(opts)

	assert.Contains(t, content, "## Language Version")
	assert.Contains(t, content, "Go 1.24 detected")
	assert.Contains(t, content, "slices_package (1.21+)")
	assert.Contains(t, content, "range_over_int (1.22+)")
	assert.Contains(t, content, "range_over_func (1.23+)")
	assert.Contains(t, content, "os_root (1.24+)")
	assert.NotContains(t, content, "weak_pointers")
}

func TestBuildIndexContent_WithoutFeatures(t *testing.T) {
	opts := GenerateOptions{
		Architecture: "hexagonal",
		Capabilities: []string{"http-api"},
		Components: []config.Component{
			{Name: "domain", In: []string{"domain/**"}, MayDependOn: []string{}},
		},
	}

	content := buildIndexContent(opts)

	assert.NotContains(t, content, "## Language Version")
}

func TestCategoryContent_HasPathsFrontmatter(t *testing.T) {
	opts := GenerateOptions{
		Architecture: "hexagonal",
		Capabilities: []string{"mysql"},
	}

	content := buildCategoryContent("data", []string{"mysql"}, opts)

	// Claude Code scopes a rule with a `paths:` YAML list; it ignores `globs:`,
	// which made every category file load in every session.
	assert.True(t, strings.HasPrefix(content, "---\npaths:\n  - \"internal/repository/**\"\n"), "got:\n%s", content)
	assert.Contains(t, content, "  - \"adapter/*repo/**\"\n")
	assert.NotContains(t, content, "globs:")
}

func claudeRuleNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, ".claude", "rules"))
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// A category the project no longer has must not leave its file behind: Claude
// Code would keep loading the stale rules.
func TestWriteSplit_RemovesStaleCategoryFiles(t *testing.T) {
	dir := t.TempDir()
	opts := GenerateOptions{ProjectDir: dir, Target: "claude", Architecture: "hexagonal", Capabilities: []string{"http-api", "mysql"}}
	require.NoError(t, Generate(opts))
	require.FileExists(t, filepath.Join(dir, ".claude", "rules", "verikt-data.md"))

	opts.Capabilities = []string{"http-api"}
	require.NoError(t, Generate(opts))

	assert.ElementsMatch(t, []string{"verikt-index.md", "verikt-http.md"}, claudeRuleNames(t, dir))
}

// Switching to --catalog-only replaces the split files with verikt.md, and
// switching back removes verikt.md.
func TestClaudeRules_SwitchingCatalogOnlyLeavesNoStaleFiles(t *testing.T) {
	dir := t.TempDir()
	full := GenerateOptions{ProjectDir: dir, Target: "claude", Architecture: "hexagonal", Capabilities: []string{"http-api"}}
	require.NoError(t, Generate(full))

	require.NoError(t, Generate(GenerateOptions{ProjectDir: dir, Target: "claude", CatalogOnly: true}))
	assert.ElementsMatch(t, []string{"verikt.md"}, claudeRuleNames(t, dir))

	require.NoError(t, Generate(full))
	assert.ElementsMatch(t, []string{"verikt-index.md", "verikt-http.md"}, claudeRuleNames(t, dir))
}

// Only files verikt generated are removed: a user's own rule, even one whose
// name starts with "verikt", is kept.
func TestClaudeRules_KeepsUserFiles(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".claude", "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o755))
	user := map[string]string{
		"verikt-notes.md": "# my own verikt notes\n",
		"go.md":           "# go rules\n",
	}
	for name, content := range user {
		require.NoError(t, os.WriteFile(filepath.Join(rulesDir, name), []byte(content), 0o644))
	}

	require.NoError(t, Generate(GenerateOptions{ProjectDir: dir, Target: "claude", Architecture: "hexagonal", Capabilities: []string{"http-api"}}))
	require.NoError(t, Generate(GenerateOptions{ProjectDir: dir, Target: "claude", CatalogOnly: true}))

	for name, content := range user {
		data, err := os.ReadFile(filepath.Join(rulesDir, name))
		require.NoError(t, err)
		assert.Equal(t, content, string(data))
	}
}
