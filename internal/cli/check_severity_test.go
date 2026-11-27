package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/rules"
)

// writeTodoRuleProject creates a TypeScript project with one grep proxy rule of
// the given severity that matches src/app.ts.
func writeTodoRuleProject(t *testing.T, severity, extraYAML string) string {
	t.Helper()
	dir := writeCheckProject(t, extraYAML)
	rulesDir := filepath.Join(dir, ".verikt", "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o755))
	rule := "id: no-todo\nengine: grep\ndescription: \"No TODO markers\"\nseverity: " + severity +
		"\npattern: 'TODO'\nscope:\n  - \"**/*.ts\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(rulesDir, "no-todo.yaml"), []byte(rule), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "app.ts"), []byte("// TODO: fix\nexport const a = 1;\n"), 0o644))
	return dir
}

// runProxyCheck runs only proxy rules, on the Go grep implementation, so the
// test needs no engine.
func runProxyCheck(t *testing.T, output string, flags *checkFlags) (string, error) {
	t.Helper()
	stubEngineUnavailable(t)
	flags.proxyRules = true
	var err error
	out := captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: output}, flags)
	})
	return out, err
}

// --- end to end through runCheck: terminal ---

func TestCheck_WarningOnlyRunPasses(t *testing.T) {
	dir := writeTodoRuleProject(t, "warning", "")
	out, err := runProxyCheck(t, "terminal", &checkFlags{projectPath: dir})

	require.NoError(t, err)
	assert.Contains(t, out, "0 errors, 1 warning — passed (use --strict to fail on warnings)")
}

func TestCheck_ErrorRunFails(t *testing.T) {
	dir := writeTodoRuleProject(t, "error", "")
	out, err := runProxyCheck(t, "terminal", &checkFlags{projectPath: dir})

	require.ErrorIs(t, err, ErrCheckFailed)
	assert.Contains(t, out, "1 error, 0 warnings — failed")
}

func TestCheck_StrictFailsOnWarning(t *testing.T) {
	dir := writeTodoRuleProject(t, "warning", "")
	out, err := runProxyCheck(t, "terminal", &checkFlags{projectPath: dir, strict: true})

	require.ErrorIs(t, err, ErrCheckFailed)
	assert.Contains(t, out, "0 errors, 1 warning — failed (warnings fail: --strict)")
}

func TestCheck_ConfigFailOnWarningFailsOnWarning(t *testing.T) {
	dir := writeTodoRuleProject(t, "warning", "check:\n  fail_on: warning\n")
	out, err := runProxyCheck(t, "terminal", &checkFlags{projectPath: dir})

	require.ErrorIs(t, err, ErrCheckFailed)
	assert.Contains(t, out, "0 errors, 1 warning — failed (warnings fail: check.fail_on)")
}

func TestCheck_StrictOverridesConfigFailOnError(t *testing.T) {
	dir := writeTodoRuleProject(t, "warning", "check:\n  fail_on: error\n")
	_, err := runProxyCheck(t, "terminal", &checkFlags{projectPath: dir, strict: true})

	require.ErrorIs(t, err, ErrCheckFailed)
}

// An invalid threshold is a configuration error, not a check failure: it must
// not run and report a pass at some threshold the user did not ask for.
func TestCheck_InvalidFailOnIsRejected(t *testing.T) {
	dir := writeTodoRuleProject(t, "warning", "check:\n  fail_on: info\n")
	_, err := runProxyCheck(t, "terminal", &checkFlags{projectPath: dir})

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrCheckFailed)
	assert.Contains(t, err.Error(), "check.fail_on")
}

// severity_overrides keep their meaning under the new threshold: `should`
// downgrades to a warning, which passes by default.
func TestCheck_ShouldOverrideDowngradesAnErrorToAPassingWarning(t *testing.T) {
	dir := writeTodoRuleProject(t, "error",
		"severity_overrides:\n  no-todo:\n    - severity: should\n      reason: legacy code\n")
	out, err := runProxyCheck(t, "terminal", &checkFlags{projectPath: dir})

	require.NoError(t, err)
	assert.Contains(t, out, "0 errors, 1 warning — passed")
}

// A waived finding is out of the count, so --strict does not resurrect it.
func TestCheck_IgnoreOverridePassesEvenWithStrict(t *testing.T) {
	dir := writeTodoRuleProject(t, "warning",
		"severity_overrides:\n  no-todo:\n    - severity: ignore\n      reason: not enforced here\n")
	out, err := runProxyCheck(t, "terminal", &checkFlags{projectPath: dir, strict: true})

	require.NoError(t, err)
	assert.Contains(t, out, "0 errors, 0 warnings — passed")
}

// --- end to end: JSON ---

func TestCheck_JSONWarningOnlyRunPassesWithCounts(t *testing.T) {
	dir := writeTodoRuleProject(t, "warning", "")
	out, err := runProxyCheck(t, "json", &checkFlags{projectPath: dir})

	require.NoError(t, err)
	parsed := decodeCheckJSON(t, out)
	assert.Equal(t, "pass", parsed["result"])
	assert.Equal(t, "error", parsed["fail_on"])
	assert.InDelta(t, 0, parsed["error_count"], 0)
	assert.InDelta(t, 1, parsed["warning_count"], 0)
	assert.InDelta(t, 0, parsed["info_count"], 0)
}

func TestCheck_JSONErrorRunFailsWithCounts(t *testing.T) {
	dir := writeTodoRuleProject(t, "error", "")
	out, err := runProxyCheck(t, "json", &checkFlags{projectPath: dir})

	require.ErrorIs(t, err, ErrCheckFailed)
	parsed := decodeCheckJSON(t, out)
	assert.Equal(t, "fail", parsed["result"])
	assert.InDelta(t, 1, parsed["error_count"], 0)
	assert.InDelta(t, 0, parsed["warning_count"], 0)
}

func TestCheck_JSONStrictWarningRunFails(t *testing.T) {
	dir := writeTodoRuleProject(t, "warning", "")
	out, err := runProxyCheck(t, "json", &checkFlags{projectPath: dir, strict: true})

	require.ErrorIs(t, err, ErrCheckFailed)
	parsed := decodeCheckJSON(t, out)
	assert.Equal(t, "fail", parsed["result"], "result must match the exit code")
	assert.Equal(t, "warning", parsed["fail_on"])
}

const undecidedTier2 = `decisions:
  - topic: cache
    tier: 2
`

// Decision gates go through the same decision: a Tier 2 gate is a warning.
func TestCheck_DecisionWarningPassesByDefaultAndFailsWithStrict(t *testing.T) {
	dir := writeCheckProject(t, undecidedTier2)

	var err error
	out := captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "json"}, &checkFlags{projectPath: dir, decisions: true})
	})
	require.NoError(t, err)
	parsed := decodeCheckJSON(t, out)
	assert.Equal(t, "pass", parsed["result"])
	assert.InDelta(t, 1, parsed["warning_count"], 0)

	out = captureStdout(t, func() {
		err = runCheck(&globalOptions{Output: "json"}, &checkFlags{projectPath: dir, decisions: true, strict: true})
	})
	require.ErrorIs(t, err, ErrCheckFailed)
	assert.Equal(t, "fail", decodeCheckJSON(t, out)["result"])
}

// --- evaluateCheck: the single pass/fail decision over every finding source ---

// The issue's reproduction: a scaffold whose findings were all warnings —
// function metrics, naked_goroutine, and the warning-level structure detectors
// — exited 1.
func TestEvaluateCheck_CheckerWarningsOnlyPass(t *testing.T) {
	outcome := checkOutcome{Checker: &checker.CheckResult{
		FunctionViolations: []checker.Violation{{Rule: "function_params", Severity: "warning"}},
		StructureViolations: []checker.Violation{
			{Rule: checker.RuleCmdAllowlist, Severity: "warning"},
			{Rule: checker.RuleLiveDotClaudeFixture, Severity: "warning"},
		},
		AntiPatternViolations: []checker.AntiPattern{{Name: "naked_goroutine", Severity: "warning"}},
	}}

	got := evaluateCheck(outcome, config.FailOnError)
	assert.False(t, got.Failed)
	assert.Equal(t, 0, got.Errors)
	assert.Equal(t, 4, got.Warnings)

	assert.True(t, evaluateCheck(outcome, config.FailOnWarning).Failed)
}

func TestEvaluateCheck_ErrorInAnySourceFails(t *testing.T) {
	cases := map[string]checkOutcome{
		"checker":     {Checker: &checker.CheckResult{DependencyViolations: []checker.Violation{{Severity: "error"}}}},
		"antipattern": {Checker: &checker.CheckResult{AntiPatternViolations: []checker.AntiPattern{{Severity: "error"}}}},
		"proxy":       {Rules: &rules.RunResult{Violations: []rules.RuleViolation{{Severity: "error"}}}},
		"decision":    {Decisions: []checker.DecisionViolation{{Topic: "db", Tier: 1, Severity: "error"}}},
	}
	for name, outcome := range cases {
		t.Run(name, func(t *testing.T) {
			got := evaluateCheck(outcome, config.FailOnError)
			assert.True(t, got.Failed)
			assert.Equal(t, 1, got.Errors)
		})
	}
}

// Proxy rules and decisions used to fail only on the literal "error", so an
// unset severity passed there while it failed for checker findings.
func TestEvaluateCheck_UnsetSeverityCountsAsErrorInEverySource(t *testing.T) {
	cases := map[string]checkOutcome{
		"checker":  {Checker: &checker.CheckResult{StructureViolations: []checker.Violation{{Rule: "required_dir"}}}},
		"proxy":    {Rules: &rules.RunResult{Violations: []rules.RuleViolation{{RuleID: "r"}}}},
		"decision": {Decisions: []checker.DecisionViolation{{Topic: "db"}}},
	}
	for name, outcome := range cases {
		t.Run(name, func(t *testing.T) {
			got := evaluateCheck(outcome, config.FailOnError)
			assert.True(t, got.Failed)
			assert.Equal(t, 1, got.Errors)
		})
	}
}

func TestEvaluateCheck_InfoNeverFails(t *testing.T) {
	outcome := checkOutcome{
		Checker: &checker.CheckResult{AntiPatternViolations: []checker.AntiPattern{{Name: "x", Severity: "info"}}},
		Rules:   &rules.RunResult{Violations: []rules.RuleViolation{{RuleID: "r", Severity: "info"}}},
	}
	got := evaluateCheck(outcome, config.FailOnWarning)

	assert.False(t, got.Failed)
	assert.Equal(t, 2, got.Infos)
}

// A rule that could not run fails at every threshold; it is not a finding, so
// it is counted on its own.
func TestEvaluateCheck_UnrunProxyRuleFailsWithoutFindings(t *testing.T) {
	outcome := checkOutcome{Rules: &rules.RunResult{Statuses: []rules.RuleStatus{
		{Rule: rules.Rule{ID: "r"}, Filename: "r.yaml", Status: "stale"},
	}}}
	got := evaluateCheck(outcome, config.FailOnError)

	assert.True(t, got.Failed)
	assert.Equal(t, 0, got.Errors)
	assert.Equal(t, 1, got.UnrunRules)
}

// --- why the run failed ---

func TestCheckVerdictSummaryLine(t *testing.T) {
	cases := []struct {
		name string
		v    checkVerdict
		want string
	}{
		{"clean", checkVerdict{FailOn: "error"}, "0 errors, 0 warnings — passed"},
		{"warnings pass", checkVerdict{FailOn: "error", Warnings: 3},
			"0 errors, 3 warnings — passed (use --strict to fail on warnings)"},
		{"errors fail", checkVerdict{FailOn: "error", Errors: 2, Warnings: 3, Failed: true},
			"2 errors, 3 warnings — failed"},
		{"strict", checkVerdict{FailOn: "warning", FailOnSource: "--strict", Warnings: 1, Failed: true},
			"0 errors, 1 warning — failed (warnings fail: --strict)"},
		{"config", checkVerdict{FailOn: "warning", FailOnSource: "check.fail_on", Warnings: 2, Failed: true},
			"0 errors, 2 warnings — failed (warnings fail: check.fail_on)"},
		{"info", checkVerdict{FailOn: "error", Errors: 1, Infos: 2, Failed: true},
			"1 error, 0 warnings, 2 info — failed"},
		{"unrun", checkVerdict{FailOn: "error", UnrunRules: 2, Failed: true},
			"0 errors, 0 warnings — failed (2 proxy rules did not run)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.v.summaryLine())
		})
	}
}

func TestCheckPrintCombinedTerminal_EndsWithSummaryLine(t *testing.T) {
	out := captureStdout(t, func() {
		printCombinedTerminal(checkOutcome{}, &config.VeriktConfig{Architecture: "flat"}, &checkFlags{},
			checkVerdict{FailOn: "error", Errors: 1, Failed: true})
	})
	assert.Contains(t, out, "1 error, 0 warnings — failed")
}

func TestCheckPrintCombinedJSON_IncludesCountsAndFailOn(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printCombinedJSON(checkOutcome{},
			checkVerdict{FailOn: "error", Errors: 2, Warnings: 3, Failed: true}))
	})
	parsed := decodeCheckJSON(t, out)
	assert.Equal(t, "fail", parsed["result"])
	assert.Equal(t, "error", parsed["fail_on"])
	assert.InDelta(t, 2, parsed["error_count"], 0)
	assert.InDelta(t, 3, parsed["warning_count"], 0)
	assert.InDelta(t, 0, parsed["info_count"], 0)
}

// A finding with no severity fails, so it must carry the failure marker.
func TestSeverityMarker_UnsetSeverityIsAFailure(t *testing.T) {
	assert.Equal(t, "✗", severityMarker(""))
	assert.Equal(t, "✗", severityMarker("error"))
	assert.Equal(t, "⚠", severityMarker("warning"))
}
