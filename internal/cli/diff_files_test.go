package cli

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitRepo builds a throwaway repository with one commit per step and returns its
// path. Each step is a map of relative path to contents.
func gitRepo(t *testing.T, commits ...map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	run("init", "--initial-branch=main")
	for i, files := range commits {
		for rel, content := range files {
			path := filepath.Join(dir, rel)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		}
		run("add", "-A")
		run("commit", "-m", "commit")
		_ = i
	}
	return dir
}

// getDiffFiles must name the files that changed since the ref.
//
// It ran `git diff --name-only -- <ref>`, and that `--` makes git read the ref as
// a *pathspec*: it matches no path, returns nothing, and exits 0. Every finding
// was then filtered away and the check exited 0 — so `verikt check --diff main`,
// the documented way to adopt verikt on an existing codebase without cleaning up
// the whole repository first, passed unconditionally.
func TestGetDiffFilesNamesChangedFiles(t *testing.T) {
	repo := gitRepo(t,
		map[string]string{
			"internal/keep.go":  "package keep\n",
			"internal/touch.go": "package touch\n",
		},
		map[string]string{
			"internal/touch.go": "package touch\n\nvar Changed = true\n",
			"internal/added.go": "package added\n",
		},
	)

	files, err := getDiffFiles(repo, "HEAD~1")
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"internal/touch.go", "internal/added.go"}, files,
		"the ref must be read as a revision, not a pathspec")
}

// A branch name is the form the docs use, and it must behave the same way.
func TestGetDiffFilesAcceptsABranchName(t *testing.T) {
	repo := gitRepo(t,
		map[string]string{"internal/keep.go": "package keep\n"},
	)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "internal", "feature.go"),
		[]byte("package feature\n"), 0o644))
	run("add", "-A")
	run("commit", "-m", "feature work")

	files, err := getDiffFiles(repo, "main")
	require.NoError(t, err)

	assert.Equal(t, []string{"internal/feature.go"}, files)
}

// An unknown ref must be an error, not an empty file list. Returning nothing
// would filter every finding away and report a pass, which is the failure mode
// the pathspec bug produced for *every* ref.
func TestGetDiffFilesRejectsAnUnknownRef(t *testing.T) {
	repo := gitRepo(t, map[string]string{"internal/keep.go": "package keep\n"})

	_, err := getDiffFiles(repo, "no-such-ref")
	require.Error(t, err, "an unresolvable ref must fail rather than silently match nothing")
}

// A ref with no changes against it is legitimately empty.
func TestGetDiffFilesReturnsNothingWhenUnchanged(t *testing.T) {
	repo := gitRepo(t, map[string]string{"internal/keep.go": "package keep\n"})

	files, err := getDiffFiles(repo, "HEAD")
	require.NoError(t, err)
	assert.Empty(t, files)
}

// Each narrowing flag restricts the run to one kind of check, so combining two
// of them restricts it to nothing.
//
// `--detectors --rule x` skipped the detectors (because --rule was set) and the
// proxy rules (because --detectors was set), then printed a header with an empty
// body and exited 0 — reopening the silent pass that --rule's unknown-ID error
// was added to close.
func TestNarrowingFlagsAreMutuallyExclusive(t *testing.T) {
	combos := [][]string{
		{"--detectors", "--rule", "x"},
		{"--detectors", "--proxy-rules"},
		{"--proxy-rules", "--rule", "x"},
		{"--detectors", "--decisions"},
		{"--decisions", "--proxy-rules"},
	}

	for _, args := range combos {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := newRootCommand()
			cmd.SetArgs(append([]string{"check"}, args...))
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			require.Error(t, err, "a combination that runs no checks must not exit 0")
			assert.Contains(t, err.Error(), "none of the others can be",
				"cobra should report the exclusivity, got: %v", err)
		})
	}
}

// The flags still work on their own.
func TestNarrowingFlagsWorkIndividually(t *testing.T) {
	for _, arg := range []string{"--detectors", "--proxy-rules", "--decisions"} {
		cmd := newRootCommand()
		cmd.SetArgs([]string{"check", "--path", t.TempDir(), arg})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		err := cmd.Execute()
		// No verikt.yaml in the temp dir, so this fails on configuration — the
		// point is that it is not rejected as a flag conflict.
		if err != nil {
			assert.NotContains(t, err.Error(), "none of the others can be",
				"%s alone must not be rejected as a conflict", arg)
		}
	}
}
