package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/engineclient"
)

// The Go grep path and the Rust grep path must agree on rule status. They did
// not: the Go loader derived staleness from scope expansion, while the engine
// derived it from whether a finding was emitted, so a rule that ran cleanly came
// back "stale" — and stale rules fail the build.
//
// Testing the two paths against each other is the guard that generalises. A test
// that only pinned the reported symptom would have passed while the next
// divergence shipped, which is how this one survived: engine_test.go already
// asserted the correct Go behaviour.
func TestGrepStatusParityBetweenGoAndEngine(t *testing.T) {
	enginePath, err := engineclient.EnginePath()
	if err != nil {
		t.Skipf("embedded engine unavailable on this platform: %v", err)
	}
	client := engineclient.New(enginePath)

	cases := []struct {
		name  string
		rule  string
		files map[string]string
		want  string
	}{
		{
			name: "scope matches files and rule finds nothing",
			files: map[string]string{
				"internal/agent/a.go": "package agent\n",
			},
			rule: `
id: no-sprintf
engine: grep
description: "No Sprintf"
severity: error
pattern: 'Sprintf'
scope:
  - "internal/**/*.go"
`,
			want: "valid",
		},
		{
			name: "scope matches files and rule finds a violation",
			files: map[string]string{
				"internal/agent/a.go": "package agent\n\nvar _ = Sprintf\n",
			},
			rule: `
id: no-sprintf
engine: grep
description: "No Sprintf"
severity: error
pattern: 'Sprintf'
scope:
  - "internal/**/*.go"
`,
			want: "valid",
		},
		{
			name: "scope matches no files",
			files: map[string]string{
				"internal/agent/a.go": "package agent\n",
			},
			rule: `
id: no-sprintf
engine: grep
description: "No Sprintf"
severity: error
pattern: 'Sprintf'
scope:
  - "removed/**/*.go"
`,
			want: "stale",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goStatuses := runForStatuses(t, tc.files, tc.rule, nil)
			engineStatuses := runForStatuses(t, tc.files, tc.rule, client)

			assert.Equal(t, tc.want, goStatuses["no-sprintf"], "Go path status")
			assert.Equal(t, tc.want, engineStatuses["no-sprintf"], "engine path status")
			assert.Equal(t, goStatuses, engineStatuses,
				"Go and Rust must agree on rule status — a divergence here is the bug class this guards")
		})
	}
}

func runForStatuses(t *testing.T, files map[string]string, rule string, client *engineclient.Client) map[string]string {
	t.Helper()

	dir := setupTestProject(t, files)
	rulesDir := setupRulesDir(t, dir)
	writeRule(t, rulesDir, "rule.yaml", rule)

	result, err := RunRules(rulesDir, dir, nil, client)
	require.NoError(t, err)

	statuses := make(map[string]string, len(result.Statuses))
	for _, s := range result.Statuses {
		statuses[s.Rule.ID] = s.Status
	}
	return statuses
}
