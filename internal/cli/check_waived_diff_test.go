package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/config"
)

// A waiver is part of the record of what verikt found, so --diff and --staged
// (which the PostToolUse hook uses) must keep the waivers in the changed files
// rather than dropping every one of them.
func TestFilterCheckerResultByFilesKeepsWaiversInChangedFiles(t *testing.T) {
	r := &checker.CheckResult{
		WaivedFindings: []checker.WaivedFinding{
			{Category: "structure", Rule: "silent_ci_swallow", File: "scripts/ci.sh", Line: 3, Reason: "best effort"},
			{Category: "structure", Rule: "silent_ci_swallow", File: "scripts/other.sh", Line: 9, Reason: "unchanged"},
		},
	}

	filtered := filterCheckerResultByFiles(r, []string{"scripts/ci.sh"})

	if assert.Len(t, filtered.WaivedFindings, 1) {
		assert.Equal(t, "scripts/ci.sh", filtered.WaivedFindings[0].File)
		assert.Equal(t, "best effort", filtered.WaivedFindings[0].Reason)
	}
}

// An inline verikt:allow waiver (recorded by the checker) and a
// severity_overrides waiver in the same run must both be reported. The outcome
// used to carry only the override waivers, so every inline waiver vanished from
// WAIVED and waived[] as soon as any override matched.
func TestApplySeverityOverridesKeepsInlineWaivers(t *testing.T) {
	r := &checker.CheckResult{
		StructureViolations: []checker.Violation{
			{Category: "structure", Rule: "god_package", File: "internal/big/big.go", Severity: "warning"},
		},
		WaivedFindings: []checker.WaivedFinding{
			{Category: "structure", Rule: "silent_ci_swallow", File: "scripts/ci.sh", Line: 3, Reason: "advisory"},
		},
	}
	overrides := config.SeverityOverrides{
		"god_package": {{Severity: "ignore", Reason: "legacy package"}},
	}

	outcome := checkOutcome{Checker: r, Waived: applySeverityOverrides(r, nil, overrides)}

	reasons := map[string]string{}
	for _, w := range outcome.waived() {
		reasons[w.Rule] = w.Reason
	}
	assert.Equal(t, map[string]string{"silent_ci_swallow": "advisory", "god_package": "legacy package"}, reasons)
}
