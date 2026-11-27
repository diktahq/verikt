package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCheckJSON runs check in -o json mode and returns the error and stdout.
func runCheckJSON(t *testing.T, flags *checkFlags) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "json"}, flags)
	})
	return out, err
}

// `info` is not an override severity (the vocabulary is must, should, ignore),
// and check did not validate overrides at all. An error-level finding
// overridden to info was counted as info, which never fails — so it passed
// under --strict, and because it was not "ignore" it was not in waived[]
// either: a finding switched off with no trace in the report.
//
// It is rejected rather than treated as a waiver: the save path already
// rejects it and the docs list only must, should and ignore, so there is no
// existing meaning to preserve, and "ignore" with a reason already expresses
// "accepted, do not fail" in a way the report shows.
func TestCheckRejectsInfoAsAnOverrideSeverity(t *testing.T) {
	dir := writeCheckProject(t, `severity_overrides:
  arch/domain:
    - severity: info
      reason: "quiet it"
`)

	_, err := runCheckJSON(t, &checkFlags{projectPath: dir, proxyRules: true, strict: true})

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrCheckFailed, "a configuration error, not a verdict")
	assert.Contains(t, err.Error(), `severity_overrides["arch/domain"][0].severity "info" is invalid`)
}

func TestCheckRejectsAnUnknownOverrideSeverity(t *testing.T) {
	dir := writeCheckProject(t, `severity_overrides:
  arch/domain:
    - severity: warn
      reason: "typo"
`)

	_, err := runCheckJSON(t, &checkFlags{projectPath: dir, proxyRules: true})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `"warn" is invalid`)
}

// A waiver's reason is what makes it reviewable: the docs and the code comment
// on applySeverityOverrides both say it is required, and check accepted one
// without.
func TestCheckRejectsAWaiverWithoutAReason(t *testing.T) {
	dir := writeCheckProject(t, `severity_overrides:
  arch/domain:
    - severity: ignore
      paths: ["legacy/**"]
`)

	_, err := runCheckJSON(t, &checkFlags{projectPath: dir, proxyRules: true})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `severity_overrides["arch/domain"][0].reason is required`)
}

// Raising a finding to blocking hides nothing, and the published examples
// write `severity: must` without a reason, so check keeps accepting it.
func TestCheckAcceptsMustWithoutAReason(t *testing.T) {
	dir := writeCheckProject(t, `severity_overrides:
  god_package:
    - severity: must
`)

	out, err := runCheckJSON(t, &checkFlags{projectPath: dir, proxyRules: true})

	require.NoError(t, err)
	assert.Equal(t, "pass", decodeCheckJSON(t, out)["result"])
}

// An allowlist entry that leaves the project can never name an entrypoint in
// it, so cmd_allowlist would check something other than what was written.
func TestCheckRejectsAnAllowedEntrypointOutsideTheProject(t *testing.T) {
	dir := writeCheckProject(t, `check:
  allowed_entrypoints: ["../elsewhere"]
`)

	_, err := runCheckJSON(t, &checkFlags{projectPath: dir, proxyRules: true})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not leave the project")
}

// writeWaivedProxyRuleProject is a project with one proxy rule, one finding for
// it, and a waiver covering that finding.
func writeWaivedProxyRuleProject(t *testing.T) string {
	t.Helper()
	dir := writeCheckProject(t, `severity_overrides:
  no-panic:
    - severity: ignore
      reason: "a CLI entrypoint, panics are fatal by design"
      paths: ["tool/**"]
`)
	rulesDir := filepath.Join(dir, ".verikt", "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "panic.yaml"), []byte(`id: no-panic
engine: grep
description: "panic in library code"
severity: error
pattern: 'panic\('
scope: ["**/*.ts"]
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "tool"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tool", "main.ts"), []byte("panic(\"x\")\n"), 0o644))
	return dir
}

// With only proxy rules running there is no checker result, and the waived
// proxy finding was recorded on that nil result — that is, dropped. The run
// printed "All proxy rules pass" with an empty waived[], indistinguishable
// from a rule that never ran.
func TestCheckRecordsProxyWaiversWhenOnlyProxyRulesRun(t *testing.T) {
	for name, flags := range map[string]checkFlags{
		"--proxy-rules": {proxyRules: true},
		"--rule":        {rule: "no-panic"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeWaivedProxyRuleProject(t)
			flags.projectPath = dir

			out, err := runCheckJSON(t, &flags)

			require.NoError(t, err, "a waived finding does not fail the run")
			waived, ok := decodeCheckJSON(t, out)["waived"].([]any)
			require.True(t, ok)
			require.Len(t, waived, 1, "the waived proxy finding must be in waived[]")
			assert.Equal(t, "no-panic", waived[0].(map[string]any)["rule"])
		})
	}
}

// --allow-degraded with no engine leaves the same nil checker result.
func TestCheckRecordsProxyWaiversWhenDetectorsAreSkipped(t *testing.T) {
	stubEngineUnavailable(t)
	dir := writeWaivedProxyRuleProject(t)

	var err error
	out := captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "terminal"}, &checkFlags{projectPath: dir, allowDegraded: true})
	})

	require.NoError(t, err)
	assert.Contains(t, out, "WAIVED (1)")
	assert.Contains(t, out, "panics are fatal by design")
}
