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
	"github.com/diktahq/verikt/internal/checker/scriptguard"
	"github.com/diktahq/verikt/internal/engineclient"
)

// silent_ci_swallow, mutable_install_url and invalid_allow_directive report
// through StructureViolations, like cmd_allowlist and live_dot_claude_fixture,
// so JSON, the STRUCTURE VIOLATIONS section, severity_overrides, check.exclude
// and --diff reach them through the existing paths.

// writeScriptGuardProject creates a flat Go project with a workflow that
// swallows test failures, an untagged Dockerfile base image and an allow
// marker with a typo, plus extraYAML appended to its verikt.yaml.
func writeScriptGuardProject(t *testing.T, extraYAML string) string {
	t.Helper()
	if _, err := engineclient.EnginePath(); err != nil {
		t.Skipf("verikt-engine is not available for this platform, so check cannot run: %v", err)
	}
	dir := t.TempDir()
	files := map[string]string{
		"verikt.yaml": "language: go\narchitecture: flat\ncomponents: []\n" + extraYAML,
		"go.mod":      "module example.com/demo\n\ngo 1.22\n",
		"main.go":     "package main\n\nfunc main() {}\n",
		".github/workflows/ci.yml": "jobs:\n  test:\n    steps:\n" +
			"      - run: go test ./... || true\n" +
			"      - run: echo hi # verikt:allow silent_ci_swalow reason=\"typo\"\n",
		"Dockerfile": "FROM golang:1.23 AS build\nFROM ubuntu\n",
	}
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return dir
}

func guardFindings(t *testing.T, out string) (detectorCheckJSON, map[string]checker.Violation) {
	t.Helper()
	var got detectorCheckJSON
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	byRule := map[string]checker.Violation{}
	for _, v := range got.Violations {
		switch v.Rule {
		case scriptguard.RuleSilentCISwallow, scriptguard.RuleMutableInstallURL, scriptguard.RuleInvalidAllowDirective:
			byRule[v.Rule] = v
		}
	}
	return got, byRule
}

func TestCheck_ScriptGuards_JSONReportsAllThreeRulesAsWarnings(t *testing.T) {
	dir := writeScriptGuardProject(t, "")

	out, err := runDetectorCheck(t, dir, "json")

	require.NoError(t, err, "warnings do not fail the check: %s", out)
	got, rules := guardFindings(t, out)
	assert.Equal(t, "pass", got.Result)
	require.Len(t, rules, 3, out)
	sw := rules[scriptguard.RuleSilentCISwallow]
	assert.Equal(t, ".github/workflows/ci.yml", sw.File)
	assert.Equal(t, 4, sw.Line)
	assert.Equal(t, "structure", sw.Category)
	assert.Equal(t, "Dockerfile", rules[scriptguard.RuleMutableInstallURL].File)
	assert.Equal(t, 2, rules[scriptguard.RuleMutableInstallURL].Line)
	assert.Equal(t, 5, rules[scriptguard.RuleInvalidAllowDirective].Line)
	for _, v := range rules {
		assert.Equal(t, "warning", v.Severity, v.Rule)
	}
}

func TestCheck_ScriptGuards_TerminalShowsFileAndLine(t *testing.T) {
	dir := writeScriptGuardProject(t, "")

	out, err := runDetectorCheck(t, dir, "terminal")

	require.NoError(t, err, out)
	section := out[strings.Index(out, "STRUCTURE VIOLATIONS"):]
	assert.Contains(t, section, "STRUCTURE VIOLATIONS (3)")
	assert.Contains(t, section, "⚠ .github/workflows/ci.yml:4 ")
	assert.Contains(t, section, "⚠ Dockerfile:2 ")
}

func TestCheck_ScriptGuards_StrictFailsOnThem(t *testing.T) {
	dir := writeScriptGuardProject(t, "check:\n  fail_on: warning\n")

	out, err := runDetectorCheck(t, dir, "json")

	require.ErrorIs(t, err, ErrCheckFailed, out)
}

func TestCheck_ScriptGuards_SeverityOverridesAndExclude(t *testing.T) {
	dir := writeScriptGuardProject(t, `check:
  exclude: [".github/**"]
severity_overrides:
  mutable_install_url:
    - severity: must
      reason: base images must be pinned in this service
      paths: ["Dockerfile"]
`)

	out, err := runDetectorCheck(t, dir, "json")

	require.ErrorIs(t, err, ErrCheckFailed, "a must override raises the guard to error: %s", out)
	_, rules := guardFindings(t, out)
	assert.NotContains(t, rules, scriptguard.RuleSilentCISwallow)
	assert.NotContains(t, rules, scriptguard.RuleInvalidAllowDirective)
	assert.Equal(t, "error", rules[scriptguard.RuleMutableInstallURL].Severity)
}

func TestCheck_ScriptGuards_IgnoreOverrideIsWaived(t *testing.T) {
	dir := writeScriptGuardProject(t, `severity_overrides:
  silent_ci_swallow:
    - severity: ignore
      reason: advisory workflow during the lint migration
      paths: [".github/workflows/**"]
`)

	out, err := runDetectorCheck(t, dir, "json")

	require.NoError(t, err, out)
	got, rules := guardFindings(t, out)
	assert.NotContains(t, rules, scriptguard.RuleSilentCISwallow)
	require.Len(t, got.Waived, 1, out)
	assert.Equal(t, scriptguard.RuleSilentCISwallow, got.Waived[0].Rule)
}

func TestCheck_ScriptGuards_InvalidExtraCommandsStopsTheRun(t *testing.T) {
	dir := writeScriptGuardProject(t, "check:\n  silent_ci_swallow:\n    extra_commands: [\"go test | tee\"]\n")

	_, err := runDetectorCheck(t, dir, "json")

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrCheckFailed)
	assert.Contains(t, err.Error(), "extra_commands[0]")
}

func TestFilterCheckerResultByFiles_KeepsScriptGuardsInDiff(t *testing.T) {
	r := &checker.CheckResult{StructureViolations: []checker.Violation{
		{Rule: scriptguard.RuleSilentCISwallow, File: ".github/workflows/ci.yml", Line: 4, Severity: "warning"},
		{Rule: scriptguard.RuleMutableInstallURL, File: "Dockerfile", Line: 2, Severity: "warning"},
	}}

	filtered := filterCheckerResultByFiles(r, []string{"Dockerfile"})

	require.Len(t, filtered.StructureViolations, 1)
	assert.Equal(t, scriptguard.RuleMutableInstallURL, filtered.StructureViolations[0].Rule)
}
