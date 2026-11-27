package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/engineclient"
)

// cmd_allowlist and live_dot_claude_fixture report through StructureViolations,
// so the JSON violations[] array, the STRUCTURE VIOLATIONS section,
// severity_overrides and --diff all reach them through the existing paths.
// These tests drive runCheck end to end so that stays true.

// writeDetectorProject creates a Go project with two cmd/ entrypoints and a
// live .claude/ fixture, plus extraYAML appended to its verikt.yaml. The
// .claude/ tree is built inside t.TempDir() — never commit one to this repo.
func writeDetectorProject(t *testing.T, extraYAML string) string {
	t.Helper()
	if _, err := engineclient.EnginePath(); err != nil {
		t.Skipf("verikt-engine is not available for this platform, so check cannot run: %v", err)
	}
	dir := t.TempDir()
	files := map[string]string{
		"verikt.yaml":                          "language: go\narchitecture: flat\ncomponents: []\n" + extraYAML,
		"go.mod":                               "module example.com/demo\n\ngo 1.22\n",
		"cmd/server/main.go":                   "package main\n\nfunc main() {}\n",
		"cmd/helper/main.go":                   "package main\n\nfunc main() {}\n",
		"testdata/case/.claude/rules/rule.md":  "# fixture rule\n",
		"testdata/case/dot-claude/rules/ok.md": "# stored safely\n",
	}
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return dir
}

const allowServerOnly = "check:\n  allowed_entrypoints: [cmd/server]\n"

type detectorCheckJSON struct {
	Result     string                  `json:"result"`
	Violations []checker.Violation     `json:"violations"`
	Waived     []checker.WaivedFinding `json:"waived"`
}

func runDetectorCheck(t *testing.T, dir, output string) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: output}, &checkFlags{projectPath: dir, detectors: true})
	})
	return out, err
}

func violationsByRule(vs []checker.Violation, rule string) []checker.Violation {
	var out []checker.Violation
	for _, v := range vs {
		if v.Rule == rule {
			out = append(out, v)
		}
	}
	return out
}

func TestCheck_StructureDetectorsAppearInJSONAsWarnings(t *testing.T) {
	dir := writeDetectorProject(t, allowServerOnly)

	out, err := runDetectorCheck(t, dir, "json")

	require.NoError(t, err, "warnings do not fail the check: %s", out)
	var got detectorCheckJSON
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	assert.Equal(t, "pass", got.Result)

	entry := violationsByRule(got.Violations, checker.RuleCmdAllowlist)
	require.Len(t, entry, 1, out)
	assert.Equal(t, "cmd/helper/main.go", entry[0].File)
	assert.Equal(t, "warning", entry[0].Severity)

	fixture := violationsByRule(got.Violations, checker.RuleLiveDotClaudeFixture)
	require.Len(t, fixture, 1, out)
	assert.Equal(t, "testdata/case/.claude", fixture[0].File)
	assert.Equal(t, "warning", fixture[0].Severity)
}

func TestCheck_StructureDetectorsAppearInTheStructureSection(t *testing.T) {
	dir := writeDetectorProject(t, allowServerOnly)

	out, err := runDetectorCheck(t, dir, "terminal")

	require.NoError(t, err, out)
	section := out[strings.Index(out, "STRUCTURE VIOLATIONS"):]
	assert.Contains(t, section, "STRUCTURE VIOLATIONS (2)")
	assert.Contains(t, section, "⚠ cmd/helper/main.go — unapproved entrypoint cmd/helper")
	assert.Contains(t, section, "⚠ testdata/case/.claude — ")
}

func TestCheck_EntrypointsOffWithoutAllowlist(t *testing.T) {
	dir := writeDetectorProject(t, "")

	out, err := runDetectorCheck(t, dir, "json")

	require.NoError(t, err, out)
	assert.NotContains(t, out, checker.RuleCmdAllowlist)
}

func TestCheck_SeverityOverrideRaisesEntrypointToError(t *testing.T) {
	dir := writeDetectorProject(t, allowServerOnly+`severity_overrides:
  cmd_allowlist:
    - severity: must
      reason: binary sprawl blocks the build here
`)

	out, err := runDetectorCheck(t, dir, "json")

	require.ErrorIs(t, err, ErrCheckFailed, out)
	var got detectorCheckJSON
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	entry := violationsByRule(got.Violations, checker.RuleCmdAllowlist)
	require.Len(t, entry, 1)
	assert.Equal(t, "error", entry[0].Severity)
}

func TestCheck_SeverityOverrideWaivesLiveAgentFixture(t *testing.T) {
	dir := writeDetectorProject(t, `severity_overrides:
  live_dot_claude_fixture:
    - severity: ignore
      reason: the fixture is the subject of an agent-loading test
      paths: ["testdata/**"]
`)

	out, err := runDetectorCheck(t, dir, "json")

	require.NoError(t, err, out)
	var got detectorCheckJSON
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	assert.Empty(t, violationsByRule(got.Violations, checker.RuleLiveDotClaudeFixture))
	require.Len(t, got.Waived, 1, "a waiver is reported, not dropped")
	assert.Equal(t, checker.RuleLiveDotClaudeFixture, got.Waived[0].Rule)
}

// live_dot_claude_fixture reports the directory, and a diff lists the files in
// it. Matching by file only meant --diff dropped the finding in the one change
// that introduced the fixture.
func TestFilterCheckerResultByFiles_KeepsDirectoryFindingWhenAFileInsideChanged(t *testing.T) {
	r := &checker.CheckResult{StructureViolations: []checker.Violation{
		{Rule: checker.RuleLiveDotClaudeFixture, File: "testdata/new/.claude", Severity: "warning"},
		{Rule: checker.RuleLiveDotClaudeFixture, File: "testdata/old/.claude", Severity: "warning"},
		{Rule: checker.RuleCmdAllowlist, File: "cmd/helper/main.go", Severity: "warning"},
		{Rule: checker.RuleCmdAllowlist, File: "cmd/old/main.go", Severity: "warning"},
	}}

	filtered := filterCheckerResultByFiles(r, []string{
		"testdata/new/.claude/rules/r.md",
		"testdata/new/.claude-notes.md",
		"cmd/helper/main.go",
	})

	files := make([]string, 0, len(filtered.StructureViolations))
	for _, v := range filtered.StructureViolations {
		files = append(files, v.File)
	}
	assert.Equal(t, []string{"testdata/new/.claude", "cmd/helper/main.go"}, files)
}
