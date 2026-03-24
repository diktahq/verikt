package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- VeriktYAML edge cases ---

func TestLoadVeriktYAML_MalformedYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "verikt.yaml")
	require.NoError(t, os.WriteFile(path, []byte(":\n  bad:\n  - [unmatched"), 0o644))

	_, err := LoadVeriktYAML(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse verikt.yaml")
}

func TestLoadVeriktYAML_ValidButEmptyComponents(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "verikt.yaml")
	content := "language: go\narchitecture: flat\ncomponents: []\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := LoadVeriktYAML(path)
	require.NoError(t, err)
	assert.Equal(t, "go", cfg.Language)
	assert.Equal(t, "flat", cfg.Architecture)
	assert.Empty(t, cfg.Components)
}

func TestFindVeriktYAML_FileDoesNotExist(t *testing.T) {
	tmp := t.TempDir()

	_, err := FindVeriktYAML(tmp)
	assert.Error(t, err)
}

func TestFindVeriktYAML_FileInCurrentDir(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "verikt.yaml")
	require.NoError(t, os.WriteFile(path, []byte("language: go\n"), 0o644))

	found, err := FindVeriktYAML(tmp)
	require.NoError(t, err)
	assert.Equal(t, path, found)
}

func TestLoadVeriktYAML_DuplicateComponentNames(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "verikt.yaml")
	content := `language: go
architecture: hexagonal
components:
  - name: domain
    in: ["domain/**"]
  - name: domain
    in: ["other/**"]
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := LoadVeriktYAML(path)
	require.NoError(t, err, "LoadVeriktYAML should not validate duplicates")
	assert.Len(t, cfg.Components, 2)
}

func TestLoadVeriktYAML_EmptyArchitecture(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "verikt.yaml")
	content := "language: go\narchitecture: \"\"\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := LoadVeriktYAML(path)
	require.NoError(t, err)
	assert.Empty(t, cfg.Architecture)
}
