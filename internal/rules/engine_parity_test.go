package rules

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/engineclient"
	pb "github.com/diktahq/verikt/internal/engineclient/pb"
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
			goRun := runForParity(t, tc.files, tc.rule, nil)
			engineRun := runForParity(t, tc.files, tc.rule, client)

			assert.Equal(t, tc.want, goRun.statuses["no-sprintf"], "Go path status")
			assert.Equal(t, tc.want, engineRun.statuses["no-sprintf"], "engine path status")
			assert.Equal(t, goRun.statuses, engineRun.statuses,
				"Go and Rust must agree on rule status — a divergence here is the bug class this guards")
			assert.Equal(t, goRun.findings, engineRun.findings,
				"Go and Rust must agree on findings, not merely on status")
		})
	}
}

// Which files each implementation walks is part of the contract, and comparing
// status alone could not see it.
//
// The engine's grep walker skipped testdata, target and `_`-prefixed directories
// while the Go walker skipped only vendor and node_modules, so the two reported
// different findings for identical input — and which one ran depended on whether
// the embedded engine resolved. Both still called the rule "valid", so a
// status-only comparison was blind to it, and none of the fixtures above contains
// such a directory.
func TestGrepFindingParityBetweenGoAndEngine(t *testing.T) {
	enginePath, err := engineclient.EnginePath()
	if err != nil {
		t.Skipf("embedded engine unavailable on this platform: %v", err)
	}
	client := engineclient.New(enginePath)

	const rule = `
id: no-sprintf
engine: grep
description: "No Sprintf"
severity: error
pattern: 'Sprintf'
scope:
  - "**/*.go"
`

	// One file per directory the two walkers might disagree about.
	files := map[string]string{
		"internal/real.go":              "package p\n\nvar _ = Sprintf\n",
		"internal/testdata/fixture.go":  "package q\n\nvar _ = Sprintf\n",
		"target/generated.go":           "package r\n\nvar _ = Sprintf\n",
		"_scratch/old.go":               "package s\n\nvar _ = Sprintf\n",
		"vendor/dep/dep.go":             "package t\n\nvar _ = Sprintf\n",
		"node_modules/pkg/index.go":     "package u\n\nvar _ = Sprintf\n",
		"internal/nested/testdata/x.go": "package v\n\nvar _ = Sprintf\n",
	}

	goRun := runForParity(t, files, rule, nil)
	engineRun := runForParity(t, files, rule, client)

	assert.Equal(t, []string{"internal/real.go"}, goRun.findings,
		"only real source is in scope on the Go path")
	assert.Equal(t, goRun.findings, engineRun.findings,
		"the two walkers must agree on which files are project source")
	assert.Equal(t, goRun.statuses, engineRun.statuses)
}

type parityRun struct {
	statuses map[string]string
	findings []string
}

func runForParity(t *testing.T, files map[string]string, rule string, client *engineclient.Client) parityRun {
	t.Helper()

	dir := setupTestProject(t, files)
	rulesDir := setupRulesDir(t, dir)
	writeRule(t, rulesDir, "rule.yaml", rule)

	result, err := RunRules(rulesDir, dir, nil, client)
	require.NoError(t, err)

	run := parityRun{statuses: make(map[string]string, len(result.Statuses))}
	for _, s := range result.Statuses {
		run.statuses[s.Rule.ID] = s.Status
	}
	for _, v := range result.Violations {
		run.findings = append(run.findings, v.File)
	}
	sort.Strings(run.findings)
	return run
}

// applyEngineStatuses must propagate the engine's verdict onto the loader's.
//
// End-to-end parity cannot guard this. The loader filters stale rules out before
// dispatch, so the engine only ever sees rules the loader considers runnable, and
// now that both use the same glob semantics the two agree — which means
// discarding the engine's statuses entirely changes no observable behaviour and no
// scenario test can see it. What the function is *for* is the case where the
// engine disagrees: it ran the rule, and if it could not reach any file or
// rejected the pattern, the rule did not run whatever the loader concluded.
//
// So it is tested directly. Deleting the body now fails.
func TestApplyEngineStatusesOverridesTheLoaderVerdict(t *testing.T) {
	result := &RunResult{
		Statuses: []RuleStatus{
			{Rule: Rule{ID: "reached-nothing"}, Filename: "a.yaml", Status: "valid"},
			{Rule: Rule{ID: "bad-pattern"}, Filename: "b.yaml", Status: "valid"},
			{Rule: Rule{ID: "agreed"}, Filename: "c.yaml", Status: "valid"},
			{Rule: Rule{ID: "not-sent"}, Filename: "d.yaml", Status: "valid"},
		},
	}

	applyEngineStatuses(result, map[string]pb.RuleStatus_Status{
		"reached-nothing": pb.RuleStatus_STALE,
		"bad-pattern":     pb.RuleStatus_INVALID,
		"agreed":          pb.RuleStatus_VALID,
	})

	byID := make(map[string]RuleStatus, len(result.Statuses))
	for _, s := range result.Statuses {
		byID[s.Rule.ID] = s
	}

	assert.Equal(t, "stale", byID["reached-nothing"].Status,
		"the engine could reach no file, so the rule did not run")
	assert.NotEmpty(t, byID["reached-nothing"].Error, "a stale rule must say why")

	assert.Equal(t, "invalid", byID["bad-pattern"].Status,
		"the engine rejected the rule, so it did not run")
	assert.NotEmpty(t, byID["bad-pattern"].Error)

	assert.Equal(t, "valid", byID["agreed"].Status, "agreement leaves the verdict alone")
	assert.Equal(t, "valid", byID["not-sent"].Status,
		"a rule the engine never reported on keeps the loader's verdict")
}
