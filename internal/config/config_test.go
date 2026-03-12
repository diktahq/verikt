package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ArchwayYAML edge cases ---

func TestLoadArchwayYAML_MalformedYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "archway.yaml")
	require.NoError(t, os.WriteFile(path, []byte(":\n  bad:\n  - [unmatched"), 0o644))

	_, err := LoadArchwayYAML(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse archway.yaml")
}

func TestLoadArchwayYAML_ValidButEmptyComponents(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "archway.yaml")
	content := "language: go\narchitecture: flat\ncomponents: []\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := LoadArchwayYAML(path)
	require.NoError(t, err)
	assert.Equal(t, "go", cfg.Language)
	assert.Equal(t, "flat", cfg.Architecture)
	assert.Empty(t, cfg.Components)
}

func TestFindArchwayYAML_FileDoesNotExist(t *testing.T) {
	tmp := t.TempDir()

	_, err := FindArchwayYAML(tmp)
	assert.Error(t, err)
}

func TestFindArchwayYAML_FileInCurrentDir(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "archway.yaml")
	require.NoError(t, os.WriteFile(path, []byte("language: go\n"), 0o644))

	found, err := FindArchwayYAML(tmp)
	require.NoError(t, err)
	assert.Equal(t, path, found)
}

func TestLoadArchwayYAML_DuplicateComponentNames(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "archway.yaml")
	content := `language: go
architecture: hexagonal
components:
  - name: domain
    in: ["domain/**"]
  - name: domain
    in: ["other/**"]
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := LoadArchwayYAML(path)
	require.NoError(t, err, "LoadArchwayYAML should not validate duplicates")
	assert.Len(t, cfg.Components, 2)
}

func TestLoadArchwayYAML_EmptyArchitecture(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "archway.yaml")
	content := "language: go\narchitecture: \"\"\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := LoadArchwayYAML(path)
	require.NoError(t, err)
	assert.Empty(t, cfg.Architecture)
}
