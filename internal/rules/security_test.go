package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandScope_SkipsSymlinks(t *testing.T) {
	// Create a project with a symlinked directory that should be excluded.
	root := t.TempDir()

	// Real source file.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main"), 0o644))

	// External directory (outside project) that we symlink into.
	external := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(external, "external.go"), []byte("package ext"), 0o644))

	// Create symlink: root/linked -> external
	require.NoError(t, os.Symlink(external, filepath.Join(root, "linked")))

	files, err := ExpandScope([]string{"**/*.go"}, nil, root, nil)
	require.NoError(t, err)

	// Should find src/main.go but NOT linked/external.go.
	assert.Contains(t, files, "src/main.go")
	for _, f := range files {
		assert.False(t, strings.HasPrefix(f, "linked/"),
			"symlinked directory should be skipped (INV-002), found: %s", f)
	}
}
