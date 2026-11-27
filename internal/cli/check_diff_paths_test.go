package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The PostToolUse hook runs `verikt check --diff HEAD` after every Write. A file
// the agent just created is untracked, and `git diff` never lists untracked
// files, so every finding in it was filtered away and the hook stayed silent on
// exactly the files an agent writes from scratch.
func TestCheckGetDiffFilesIncludesUntrackedFiles(t *testing.T) {
	repo := gitRepo(t, map[string]string{"domain/old.go": "package domain\n"})
	require.NoError(t, os.WriteFile(filepath.Join(repo, "domain", "new.go"), []byte("package domain\n"), 0o644))

	files, err := getDiffFiles(repo, "HEAD")
	require.NoError(t, err)
	assert.Equal(t, []string{"domain/new.go"}, files)
}

// Untracked files git ignores are not the project's code, and listing them would
// pull build output into the diff.
func TestCheckGetDiffFilesSkipsIgnoredUntrackedFiles(t *testing.T) {
	repo := gitRepo(t, map[string]string{".gitignore": "build/\n", "a.go": "package a\n"})
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "build"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "build", "gen.go"), []byte("package build\n"), 0o644))

	files, err := getDiffFiles(repo, "HEAD")
	require.NoError(t, err)
	assert.Empty(t, files)
}

// A project in a subdirectory of the repository reports findings relative to
// itself (domain/d.go), while git printed paths relative to the repository root
// (services/api/domain/d.go). Nothing matched, so `--diff main` in a monorepo
// passed with a real violation in the diff.
func TestCheckGetDiffFilesIsRelativeToAProjectInASubdirectory(t *testing.T) {
	repo := gitRepo(t,
		map[string]string{"services/api/domain/d.go": "package domain\n", "services/web/w.go": "package web\n"},
		map[string]string{"services/api/domain/d.go": "package domain\n\nvar X = 1\n", "services/web/w.go": "package web\n\nvar Y = 1\n"},
	)
	project := filepath.Join(repo, "services", "api")
	require.NoError(t, os.WriteFile(filepath.Join(project, "domain", "new.go"), []byte("package domain\n"), 0o644))

	files, err := getDiffFiles(project, "HEAD~1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"domain/d.go", "domain/new.go"}, files,
		"paths are relative to the checked project, and files outside it are not listed")
}

// The same mismatch made --staged, the pre-commit hook, pass in a monorepo.
func TestCheckGetStagedFilesIsRelativeToAProjectInASubdirectory(t *testing.T) {
	repo := gitRepo(t, map[string]string{"services/api/domain/d.go": "package domain\n", "other.go": "package other\n"})
	require.NoError(t, os.WriteFile(filepath.Join(repo, "services/api/domain/d.go"), []byte("package domain\n\nvar X = 1\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "other.go"), []byte("package other\n\nvar Y = 1\n"), 0o644))
	gitIn(t, repo, "add", "-A")

	files, err := getStagedFiles(filepath.Join(repo, "services", "api"))
	require.NoError(t, err)
	assert.Equal(t, []string{"domain/d.go"}, files)
}

// git C-quotes a path with non-ASCII bytes or special characters
// ("domain/caf\303\251.go"), which matches no finding path, so every finding in
// such a file was dropped.
func TestCheckGetDiffFilesNamesNonASCIIAndSpacedPathsVerbatim(t *testing.T) {
	repo := gitRepo(t, map[string]string{"a.go": "package a\n"})
	gitIn(t, repo, "checkout", "-b", "feature")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "domain"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "domain", "café.go"), []byte("package domain\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "domain", "two words.go"), []byte("package domain\n"), 0o644))
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-m", "add")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "domain", "naïve.go"), []byte("package domain\n"), 0o644))

	files, err := getDiffFiles(repo, "main")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"domain/café.go", "domain/two words.go", "domain/naïve.go"}, files)
}

func TestCheckGetStagedFilesNamesNonASCIIPathsVerbatim(t *testing.T) {
	repo := gitRepo(t, map[string]string{"a.go": "package a\n"})
	require.NoError(t, os.WriteFile(filepath.Join(repo, "café.go"), []byte("package a\n"), 0o644))
	gitIn(t, repo, "add", "-A")

	files, err := getStagedFiles(repo)
	require.NoError(t, err)
	assert.Equal(t, []string{"café.go"}, files)
}

// A tracked symlink replaced by a regular file is a typechange (T), which
// --diff-filter=ACMR excluded: the new file's findings were filtered away.
func TestCheckGetDiffFilesIncludesTypechanges(t *testing.T) {
	repo := gitRepo(t, map[string]string{"real.go": "package a\n"})
	require.NoError(t, os.Symlink("real.go", filepath.Join(repo, "link.go")))
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-m", "link")
	gitIn(t, repo, "checkout", "-b", "feature")
	require.NoError(t, os.Remove(filepath.Join(repo, "link.go")))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "link.go"), []byte("package a\n\nvar X = 1\n"), 0o644))
	gitIn(t, repo, "commit", "-am", "typechange")

	files, err := getDiffFiles(repo, "main")
	require.NoError(t, err)
	assert.Equal(t, []string{"link.go"}, files)
}

func TestCheckGetStagedFilesIncludesTypechanges(t *testing.T) {
	repo := gitRepo(t, map[string]string{"real.go": "package a\n"})
	require.NoError(t, os.Symlink("real.go", filepath.Join(repo, "link.go")))
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-m", "link")
	require.NoError(t, os.Remove(filepath.Join(repo, "link.go")))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "link.go"), []byte("package a\n\nvar X = 1\n"), 0o644))
	gitIn(t, repo, "add", "link.go")

	files, err := getStagedFiles(repo)
	require.NoError(t, err)
	assert.Equal(t, []string{"link.go"}, files)
}

// `--staged -o json` with nothing staged printed plain text, which a consumer
// parsing stdout as JSON cannot read.
func TestCheckStagedWithNothingStagedStillWritesJSON(t *testing.T) {
	dir := writeCheckProject(t, "")
	initGitRepo(t, dir)

	var err error
	out := captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "json"}, &checkFlags{projectPath: dir, staged: true})
	})

	require.NoError(t, err)
	parsed := decodeCheckJSON(t, out)
	assert.Equal(t, "pass", parsed["result"])
}
