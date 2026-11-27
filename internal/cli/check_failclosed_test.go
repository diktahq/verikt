package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/checker"
)

// stubEngineUnavailable makes engine resolution fail the way a read-only cache
// directory does, and captures what check writes to stderr.
func stubEngineUnavailable(t *testing.T) *bytes.Buffer {
	t.Helper()
	origPath, origStderr := enginePathFn, checkStderr
	enginePathFn = func() (string, error) {
		return "", errors.New("create engine cache dir: permission denied")
	}
	stderr := &bytes.Buffer{}
	checkStderr = stderr
	t.Cleanup(func() {
		enginePathFn = origPath
		checkStderr = origStderr
	})
	return stderr
}

// writeCheckProject writes a minimal TypeScript project, so no Go package
// loading is involved, with extra verikt.yaml content appended.
func writeCheckProject(t *testing.T, extraYAML string) string {
	t.Helper()
	dir := t.TempDir()
	yaml := "language: typescript\narchitecture: flat\ncomponents: []\n" + extraYAML
	require.NoError(t, os.WriteFile(filepath.Join(dir, "verikt.yaml"), []byte(yaml), 0o644))
	return dir
}

const undecidedTier1 = `decisions:
  - topic: database
    tier: 1
`

// decodeCheckJSON fails the test unless stdout is exactly one JSON document.
func decodeCheckJSON(t *testing.T, out string) map[string]any {
	t.Helper()
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), "stdout must be one JSON document, got %q", out)
	return parsed
}

// --- engine unavailable ---

// The engine path error used to be discarded: the detectors then failed with a
// generic "requires the embedded Rust engine", and the actual cause — a cache
// directory the process cannot write — was nowhere in the output.
func TestCheckEngineUnavailableFailsClosedAndNamesTheCause(t *testing.T) {
	stderr := stubEngineUnavailable(t)
	dir := writeCheckProject(t, "")

	var err error
	out := captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "terminal"}, &checkFlags{projectPath: dir})
	})

	require.ErrorIs(t, err, checker.ErrEngineRequired)
	assert.NotErrorIs(t, err, ErrCheckFailed)
	assert.Contains(t, err.Error(), "permission denied", "the resolution failure must be reported")
	assert.Contains(t, err.Error(), "--allow-degraded", "the error must name the escape hatch")
	assert.Empty(t, out, "no report is printed for a run that did not happen")
	assert.Empty(t, stderr.String())
}

func TestCheckEngineUnavailableAllowDegradedWarnsOnStderrAndContinues(t *testing.T) {
	stderr := stubEngineUnavailable(t)
	dir := writeCheckProject(t, "")

	var err error
	out := captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "terminal"}, &checkFlags{projectPath: dir, allowDegraded: true})
	})

	require.NoError(t, err)
	assert.Contains(t, out, "verikt check")
	assert.NotContains(t, out, "WARNING")
	warn := stderr.String()
	assert.Contains(t, warn, "WARNING")
	assert.Contains(t, warn, "--allow-degraded")
	assert.Contains(t, warn, "permission denied")
	assert.Contains(t, warn, "built-in detectors did not run")
}

// In JSON mode the warning must stay off stdout, or the document no longer parses.
func TestCheckEngineUnavailableAllowDegradedKeepsJSONStdoutPure(t *testing.T) {
	stderr := stubEngineUnavailable(t)
	dir := writeCheckProject(t, "")

	var err error
	out := captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "json"}, &checkFlags{projectPath: dir, allowDegraded: true})
	})

	require.NoError(t, err)
	assert.Equal(t, "pass", decodeCheckJSON(t, out)["result"])
	assert.Contains(t, stderr.String(), "WARNING")
}

// --detectors runs nothing but engine-backed checks. Degrading it would run no
// check at all and report a pass, so the escape hatch does not apply.
func TestCheckEngineUnavailableDetectorsOnlyFailsEvenWhenDegradedIsAllowed(t *testing.T) {
	stubEngineUnavailable(t)
	dir := writeCheckProject(t, "")

	var err error
	out := captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "terminal"},
			&checkFlags{projectPath: dir, detectors: true, allowDegraded: true})
	})

	require.ErrorIs(t, err, checker.ErrEngineRequired)
	assert.Empty(t, out)
}

// Proxy grep rules keep a Go implementation for platforms with no engine
// (ADR-006), and the report names which one ran. A proxy-rules-only run is not
// degraded, so it needs no flag and prints no warning.
func TestCheckEngineUnavailableProxyRulesOnlyRunsWithoutTheFlag(t *testing.T) {
	stderr := stubEngineUnavailable(t)
	dir := writeCheckProject(t, "")

	var err error
	captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "terminal"}, &checkFlags{projectPath: dir, proxyRules: true})
	})

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
}

// --decisions reads verikt.yaml only, so an unavailable engine is irrelevant.
func TestCheckDecisionsModeDoesNotResolveTheEngine(t *testing.T) {
	orig := enginePathFn
	enginePathFn = func() (string, error) {
		t.Error("decisions mode must not resolve the engine")
		return "", errors.New("unreachable")
	}
	t.Cleanup(func() { enginePathFn = orig })
	dir := writeCheckProject(t, "")

	var err error
	captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "terminal"}, &checkFlags{projectPath: dir, decisions: true})
	})
	require.NoError(t, err)
}

// --- JSON exit code ---

func TestCheckJSONDecisionsModeFailureReturnsErrCheckFailed(t *testing.T) {
	dir := writeCheckProject(t, undecidedTier1)

	var err error
	out := captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "json"}, &checkFlags{projectPath: dir, decisions: true})
	})

	require.ErrorIs(t, err, ErrCheckFailed)
	parsed := decodeCheckJSON(t, out)
	assert.Equal(t, "fail", parsed["result"])
	assert.NotEmpty(t, parsed["decision_violations"])
}

func TestCheckJSONFullModeFailureReturnsErrCheckFailed(t *testing.T) {
	stderr := stubEngineUnavailable(t)
	dir := writeCheckProject(t, undecidedTier1)

	var err error
	out := captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "json"}, &checkFlags{projectPath: dir, allowDegraded: true})
	})

	require.ErrorIs(t, err, ErrCheckFailed)
	assert.Equal(t, "fail", decodeCheckJSON(t, out)["result"])
	assert.Contains(t, stderr.String(), "WARNING")
}

// --- --diff ref handling ---

// A ref starting with '-' would reach git as an option. It is rejected before
// git runs, so `--diff=--output=x` cannot make git write a file.
func TestCheckGetDiffFilesRejectsOptionLikeRefs(t *testing.T) {
	repo := gitRepo(t, map[string]string{"a.go": "package a\n"})

	for _, ref := range []string{"--output=x", "-p", "--all", ""} {
		_, err := getDiffFiles(repo, ref)
		require.Error(t, err, "ref %q must be rejected", ref)
		if ref != "" {
			assert.Contains(t, err.Error(), "must not start with '-'")
		}
	}
	_, statErr := os.Stat(filepath.Join(repo, "x"))
	assert.True(t, os.IsNotExist(statErr), "--output=x must never reach git")
}

// A ref that is also the name of a file in the tree must still be read as a
// revision.
func TestCheckGetDiffFilesReadsARefThatIsAlsoAFileNameAsARevision(t *testing.T) {
	repo := gitRepo(t,
		map[string]string{"main": "a file named like the branch\n", "a.go": "package a\n"},
		map[string]string{"a.go": "package a\n\nvar X = 1\n"},
	)

	files, err := getDiffFiles(repo, "HEAD~1")
	require.NoError(t, err)
	assert.Equal(t, []string{"a.go"}, files)

	files, err = getDiffFiles(repo, "main")
	require.NoError(t, err)
	assert.Empty(t, files, "main is HEAD here; the file named main must not be read as a pathspec")
}

// A file renamed and edited on the branch carries findings like any other
// change. --diff-filter=ACM excluded renames (R), so every finding in a moved
// file was filtered away.
func TestCheckGetDiffFilesIncludesRenamedFiles(t *testing.T) {
	repo := gitRepo(t, map[string]string{"old/name.go": "package old\n\nfunc A() {}\nfunc B() {}\nfunc C() {}\n"})
	gitIn(t, repo, "checkout", "-b", "feature")
	gitIn(t, repo, "mv", "old/name.go", "new.go")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "new.go"),
		[]byte("package old\n\nfunc A() {}\nfunc B() {}\nfunc C() {}\nfunc D() {}\n"), 0o644))
	gitIn(t, repo, "commit", "-am", "move")

	files, err := getDiffFiles(repo, "main")
	require.NoError(t, err)
	assert.Equal(t, []string{"new.go"}, files)
}

// The same filter on --staged skipped a staged rename, so the pre-commit hook
// did not check a file the commit was about to move.
func TestCheckGetStagedFilesIncludesRenamedFiles(t *testing.T) {
	repo := gitRepo(t, map[string]string{"old/name.go": "package old\n\nfunc A() {}\nfunc B() {}\n"})
	gitIn(t, repo, "mv", "old/name.go", "new.go")

	files, err := getStagedFiles(repo)
	require.NoError(t, err)
	assert.Equal(t, []string{"new.go"}, files)
}

func gitIn(t *testing.T, dir string, args ...string) {
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
