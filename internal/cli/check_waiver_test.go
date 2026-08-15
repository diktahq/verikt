package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/rules"
)

// The upgrade notes and the CI guide both tell users to "fix them or scope them
// with severity_overrides", and anti-patterns were the one category the
// overrides deliberately skipped. A project whose pipeline newly failed had no
// per-rule escape at all — only check.exclude, which is path-scoped and drops
// every finding for that path, SQL injection included.
//
// A waiver still costs something: config validation requires a reason, so the
// justification lands in the diff where a reviewer sees it.
func TestSeverityOverridesApplyToAntiPatterns(t *testing.T) {
	result := &checker.CheckResult{
		AntiPatternViolations: []checker.AntiPattern{
			{Name: "god_package", Severity: "warning", File: "internal/core/rules.go", Message: "72 exported symbols"},
			{Name: "sql_concatenation", Severity: "error", File: "internal/repo/db.go", Message: "concatenated SQL"},
		},
	}

	overrides := config.SeverityOverrides{
		"god_package": []config.SeverityOverride{{
			Severity: "ignore",
			Reason:   "the shared domain vocabulary; 27 types + 15 constants, nothing to trim",
			Paths:    []string{"internal/core/**"},
		}},
	}

	applySeverityOverrides(result, nil, overrides)

	require.Len(t, result.AntiPatternViolations, 1, "the waived detector must not stay in the blocking set")
	assert.Equal(t, "sql_concatenation", result.AntiPatternViolations[0].Name,
		"a detector with no waiver is untouched")

	require.Len(t, result.WaivedFindings, 1, "a waiver is recorded, not silently dropped")
	waived := result.WaivedFindings[0]
	assert.Equal(t, "god_package", waived.Rule)
	assert.Equal(t, "internal/core/rules.go", waived.File)
	assert.Contains(t, waived.Reason, "shared domain vocabulary",
		"the recorded reason is the author's, so the report says why")
}

// A waiver is scoped by path like every other override: naming the detector does
// not silence it everywhere.
func TestAntiPatternWaiverIsPathScoped(t *testing.T) {
	result := &checker.CheckResult{
		AntiPatternViolations: []checker.AntiPattern{
			{Name: "god_package", Severity: "warning", File: "internal/core/rules.go"},
			{Name: "god_package", Severity: "warning", File: "internal/cli/check.go"},
		},
	}

	applySeverityOverrides(result, nil, config.SeverityOverrides{
		"god_package": []config.SeverityOverride{{
			Severity: "ignore",
			Reason:   "the domain vocabulary",
			Paths:    []string{"internal/core/**"},
		}},
	})

	require.Len(t, result.AntiPatternViolations, 1)
	assert.Equal(t, "internal/cli/check.go", result.AntiPatternViolations[0].File,
		"only the named path is waived")
}

// Raising an advisory detector to blocking is the other direction of the same
// key, and it must reach the exit code.
func TestAntiPatternSeverityCanBeRaised(t *testing.T) {
	result := &checker.CheckResult{
		AntiPatternViolations: []checker.AntiPattern{
			{Name: "god_package", Severity: "warning", File: "internal/core/rules.go"},
		},
	}

	applySeverityOverrides(result, nil, config.SeverityOverrides{
		"god_package": []config.SeverityOverride{{
			Severity: "must",
			Reason:   "we hold the line on package size in this repo",
		}},
	})

	require.Len(t, result.AntiPatternViolations, 1)
	assert.Equal(t, "error", result.AntiPatternViolations[0].Severity)
	assert.True(t, hasBlockingFindings(result), "a raised anti-pattern fails the build")
}

// Waived findings must not reach the exit code — that is the whole point.
func TestWaivedFindingsDoNotBlock(t *testing.T) {
	result := &checker.CheckResult{
		AntiPatternViolations: []checker.AntiPattern{
			{Name: "sql_concatenation", Severity: "error", File: "internal/legacy/db.go"},
		},
	}

	applySeverityOverrides(result, nil, config.SeverityOverrides{
		"sql_concatenation": []config.SeverityOverride{{
			Severity: "ignore",
			Reason:   "legacy query builder, tracked in #412, scheduled for removal",
			Paths:    []string{"internal/legacy/**"},
		}},
	})

	assert.False(t, hasBlockingFindings(result), "a waived finding does not fail the build")
	require.Len(t, result.WaivedFindings, 1)
}

// The output format must not change the verdict. `--output json` returned
// straight after printing, so a run reporting "result": "fail" still exited 0 —
// and every CI example in the docs pipes JSON to a file, so anyone gating on the
// exit code got a silent pass on exactly the runs that found something.
func TestJSONOutputStillFailsTheBuild(t *testing.T) {
	result := &checker.CheckResult{
		DependencyViolations: []checker.Violation{
			{Rule: "orphan_package", Severity: "error", File: "internal/stray", Message: "matches no component"},
		},
	}

	var err error
	captureStdout(t, func() {
		err = reportCheck(checkOutcome{Checker: result}, nil, &checkFlags{}, "json")
	})

	require.ErrorIs(t, err, ErrCheckFailed,
		"a failing check must exit non-zero whichever format it printed in")
}

func TestJSONOutputPassesWhenClean(t *testing.T) {
	var err error
	captureStdout(t, func() {
		err = reportCheck(checkOutcome{Checker: &checker.CheckResult{}}, nil, &checkFlags{}, "json")
	})

	require.NoError(t, err)
}

// A waiver that is recorded but never shown is the same as a silent drop.
func TestWaivedFindingsArePrinted(t *testing.T) {
	out := captureStdout(t, func() {
		printWaivedSection([]checker.WaivedFinding{{
			Category: "anti_pattern",
			Rule:     "god_package",
			File:     "internal/core/rules.go",
			Message:  "72 exported symbols",
			Reason:   "the shared domain vocabulary; nothing to trim",
		}})
	})

	assert.Contains(t, out, "WAIVED")
	assert.Contains(t, out, "god_package")
	assert.Contains(t, out, "internal/core/rules.go")
	assert.Contains(t, out, "the shared domain vocabulary",
		"the reason is what makes a waiver reviewable")
}

// A rule ID that matches nothing is the strongest form of "did not run".
//
// --rule with a typo filtered every rule away and then reported "All proxy rules
// pass", exit 0. A pipeline pinned to a renamed or deleted rule would go on
// passing indefinitely while enforcing nothing — the same silent-degradation
// failure as a stale rule, arriving by a different route.
func TestFilterRuleResultRejectsAnUnknownRuleID(t *testing.T) {
	result := &rules.RunResult{
		Statuses: []rules.RuleStatus{
			{Rule: rules.Rule{ID: "no-sprintf"}, Filename: "r.yaml", Status: "valid"},
		},
	}

	_, err := filterRuleResult(result, "does-not-exist")
	require.Error(t, err, "a rule ID that matches no rule must not be reported as passing")
	assert.Contains(t, err.Error(), "does-not-exist")
	assert.Contains(t, err.Error(), "no-sprintf", "the error should name the rules that do exist")

	filtered, err := filterRuleResult(result, "no-sprintf")
	require.NoError(t, err)
	assert.Len(t, filtered.Statuses, 1)
}

// A waived proxy-rule finding must be recorded, not dropped.
//
// The checker branch of applySeverityOverrides records a WaivedFinding; the proxy
// branch just skipped the violation, so the run printed "✓ All proxy rules pass"
// and the waived[] array was empty. That contradicts this file's own contract and
// the published docs, and it is indistinguishable from a rule that never ran.
//
// Every other test here passes nil as the ruleResult, which is why the branch had
// no coverage at all.
func TestWaivedProxyRuleFindingsAreRecorded(t *testing.T) {
	checkerResult := &checker.CheckResult{}
	ruleResult := &rules.RunResult{
		Statuses: []rules.RuleStatus{
			{Rule: rules.Rule{ID: "no-sprintf-sql"}, Filename: "sql.yaml", Status: "valid"},
		},
		Violations: []rules.RuleViolation{
			{RuleID: "no-sprintf-sql", Severity: "error", File: "internal/legacy/q.go", Line: 7, Description: "concatenated SQL"},
			{RuleID: "no-sprintf-sql", Severity: "error", File: "internal/repo/q.go", Line: 3, Description: "concatenated SQL"},
		},
	}

	applySeverityOverrides(checkerResult, ruleResult, config.SeverityOverrides{
		"no-sprintf-sql": []config.SeverityOverride{{
			Severity: "ignore",
			Reason:   "legacy query builder, tracked in #412",
			Paths:    []string{"internal/legacy/**"},
		}},
	})

	require.Len(t, ruleResult.Violations, 1, "only the waived path leaves the blocking set")
	assert.Equal(t, "internal/repo/q.go", ruleResult.Violations[0].File)

	require.Len(t, checkerResult.WaivedFindings, 1, "the waived proxy finding must be recorded")
	waived := checkerResult.WaivedFindings[0]
	assert.Equal(t, "proxy_rule", waived.Category)
	assert.Equal(t, "no-sprintf-sql", waived.Rule)
	assert.Equal(t, "internal/legacy/q.go", waived.File)
	assert.Equal(t, 7, waived.Line)
	assert.Contains(t, waived.Reason, "tracked in #412")
}

// And the waived finding must reach the report, whichever format is asked for.
func TestWaivedProxyRuleFindingIsReported(t *testing.T) {
	checkerResult := &checker.CheckResult{}
	ruleResult := &rules.RunResult{
		Statuses: []rules.RuleStatus{
			{Rule: rules.Rule{ID: "no-panic"}, Filename: "panic.yaml", Status: "valid"},
		},
		Violations: []rules.RuleViolation{
			{RuleID: "no-panic", Severity: "error", File: "internal/tool/main.go", Line: 12, Description: "panic in library code"},
		},
	}

	applySeverityOverrides(checkerResult, ruleResult, config.SeverityOverrides{
		"no-panic": []config.SeverityOverride{{
			Severity: "ignore",
			Reason:   "a CLI entrypoint, panics are fatal by design",
			Paths:    []string{"internal/tool/**"},
		}},
	})

	var err error
	out := captureStdout(t, func() {
		err = reportCheck(checkOutcome{Checker: checkerResult, Rules: ruleResult},
			&config.VeriktConfig{Architecture: "hexagonal"}, &checkFlags{}, "terminal")
	})

	require.NoError(t, err, "a waived finding does not fail the build")
	assert.Contains(t, out, "WAIVED")
	assert.Contains(t, out, "no-panic")
	assert.Contains(t, out, "panics are fatal by design")
}
