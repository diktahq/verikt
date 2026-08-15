package rules

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// INV-002 says every symlink is external, not only symlinked directories, and
// this walker only tested directories: a symlink to a file outside the project
// was pulled into proxy-rule scope and grepped. The invariant was widened to
// cover files in the same commit that left this line alone, and its "Proven by"
// list named a test whose fixture contains no symlink at all.
func TestExpandScopeSkipsSymlinkedFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevation on Windows")
	}

	base := t.TempDir()
	project := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(project, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(project, "real.go"), []byte("package p\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "external.go"), []byte("package q\n"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "external.go"), filepath.Join(project, "linked.go")))

	files, err := ExpandScope([]string{"**/*.go"}, nil, project, nil)
	require.NoError(t, err)

	require.Equal(t, []string{"real.go"}, files,
		"a symlinked file is not project-local code (INV-002)")
}

// The directory case, which was already handled — pinned so the widened guard
// cannot regress it while fixing the file case.
func TestExpandScopeSkipsSymlinkedDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevation on Windows")
	}

	base := t.TempDir()
	project := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(project, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(project, "real.go"), []byte("package p\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "external.go"), []byte("package q\n"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(project, "linked")))

	files, err := ExpandScope([]string{"**/*.go"}, nil, project, nil)
	require.NoError(t, err)

	require.Equal(t, []string{"real.go"}, files,
		"a symlinked directory is not project-local code (INV-002)")
}
