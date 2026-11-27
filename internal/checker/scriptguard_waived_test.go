package checker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An inline verikt:allow is a waiver, so verikt check must show it with its
// reason. A finding that vanishes entirely is indistinguishable from one that
// was never detected.
func TestCheckWithEngine_RecordsInlineAllowAsWaivedFinding(t *testing.T) {
	for _, lang := range []string{"go", "typescript"} {
		t.Run(lang, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectFile(t, dir, "scripts/ci.sh",
				"#!/bin/sh\n# verikt:allow silent_ci_swallow reason=\"advisory until #412\"\ngo test ./... || true\n")
			cfg := entrypointCfg()
			cfg.Check.AllowedEntrypoints = nil
			cfg.Language = lang

			result, err := CheckWithEngine(cfg, dir,
				emptyAntiPatternClient, dependencyClientReturning(nil), emptyMetricClient)

			require.NoError(t, err)
			for _, v := range result.StructureViolations {
				assert.NotEqual(t, "silent_ci_swallow", v.Rule, "a waived finding must not stay active")
			}
			require.Len(t, result.WaivedFindings, 1)
			w := result.WaivedFindings[0]
			assert.Equal(t, WaivedFinding{
				Category: "structure", Rule: "silent_ci_swallow", File: "scripts/ci.sh", Line: 3,
				Message: w.Message, Reason: "advisory until #412",
			}, w)
			assert.NotEmpty(t, w.Message)
		})
	}
}
