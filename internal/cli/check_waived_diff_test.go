package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/diktahq/verikt/internal/checker"
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
