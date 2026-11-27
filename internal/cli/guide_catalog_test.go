package cli

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LLM-F6: --catalog-only in a project with verikt.yaml deleted
// .verikt/references/architecture.md and stripped the layer rules from
// AGENTS.md. The global rule tells agents to run it when a user asks about
// capabilities, so the configured guide was regressed by an innocent question.
func TestRunGuide_CatalogOnlyRefusesConfiguredProject(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile("verikt.yaml",
		[]byte("language: go\narchitecture: hexagonal\ncapabilities:\n  - http-api\n"), 0o644))
	require.NoError(t, runGuide(io.Discard, "agents", false))
	before := readTree(t, dir)
	require.Contains(t, before, filepath.Join(".verikt", "references", "architecture.md"))

	err := runGuide(io.Discard, "agents", true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "verikt.yaml")
	assert.Contains(t, err.Error(), ".verikt/references/capabilities.md")
	assert.Equal(t, before, readTree(t, dir), "catalog-only must leave a configured project's guide alone")
}

func TestRunGuide_CatalogOnlyWithoutConfigWritesCatalog(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	require.NoError(t, runGuide(io.Discard, "agents", true))

	assert.FileExists(t, filepath.Join(dir, ".verikt", "references", "capabilities.md"))
	assert.FileExists(t, filepath.Join(dir, "AGENTS.md"))
}

func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out[rel] = string(data)
		return nil
	}))
	return out
}
