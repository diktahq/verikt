package rules

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diktahq/verikt/internal/engineclient"
)

// An engine that failed must not be answered by the other implementation.
//
// RunRules caught any engine error and quietly ran the Go grep path instead. The
// two implementations had already been shown to disagree, so the findings a user
// saw depended on whether the engine happened to work — with nothing in the
// output, the JSON or the exit code to say which had run. That is exactly what
// checker.ErrEngineRequired exists to prevent, and its doc comment describes this
// situation while proxy rules kept doing it.
//
// A broken engine is now an error, like it is everywhere else.
func TestEngineFailureIsNotSilentlyAnsweredByTheGoPath(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"internal/a.go": "package a\n\nvar _ = Sprintf\n",
	})
	rulesDir := setupRulesDir(t, dir)
	writeRule(t, rulesDir, "rule.yaml", `
id: no-sprintf
engine: grep
description: "No Sprintf"
severity: error
pattern: 'Sprintf'
scope:
  - "internal/**/*.go"
`)

	// A client pointed at a binary that is not an engine: Check fails.
	broken := engineclient.New("/nonexistent/verikt-engine")

	_, err := RunRules(rulesDir, dir, nil, broken)

	require.Error(t, err, "an engine that could not run must not be papered over by the Go path")
	assert.Contains(t, err.Error(), "engine",
		"the error should say the engine failed, not something generic: %v", err)
}

// With no engine at all, the Go path is allowed to run — but the result must say
// so, or a user cannot tell which implementation produced their findings.
func TestGoPathIsReportedWhenNoEngineIsAvailable(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"internal/a.go": "package a\n\nvar _ = Sprintf\n",
	})
	rulesDir := setupRulesDir(t, dir)
	writeRule(t, rulesDir, "rule.yaml", `
id: no-sprintf
engine: grep
description: "No Sprintf"
severity: error
pattern: 'Sprintf'
scope:
  - "internal/**/*.go"
`)

	result, err := RunRules(rulesDir, dir, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, GrepEngineGo, result.GrepEngine,
		"the Go implementation ran, and the result must record it")
	require.Len(t, result.Violations, 1)
}

// And when the engine runs, that is recorded too.
func TestEngineIsReportedWhenItRuns(t *testing.T) {
	enginePath, err := engineclient.EnginePath()
	if err != nil {
		t.Skipf("embedded engine unavailable on this platform: %v", err)
	}

	dir := setupTestProject(t, map[string]string{
		"internal/a.go": "package a\n\nvar _ = Sprintf\n",
	})
	rulesDir := setupRulesDir(t, dir)
	writeRule(t, rulesDir, "rule.yaml", `
id: no-sprintf
engine: grep
description: "No Sprintf"
severity: error
pattern: 'Sprintf'
scope:
  - "internal/**/*.go"
`)

	result, err := RunRules(rulesDir, dir, nil, engineclient.New(enginePath))
	require.NoError(t, err)

	assert.Equal(t, GrepEngineRust, result.GrepEngine)
	require.Len(t, result.Violations, 1)
}

// A project with no grep rules names no grep implementation, rather than
// claiming one ran.
func TestNoGrepRulesLeavesTheImplementationUnset(t *testing.T) {
	dir := setupTestProject(t, map[string]string{"internal/a.go": "package a\n"})
	rulesDir := setupRulesDir(t, dir)
	writeRule(t, rulesDir, "rule.yaml", `
id: no-global-mutable
engine: ast
description: "No global mutable state"
severity: error
detector: global_mutable_state
scope:
  - "internal/**/*.go"
`)

	result, err := RunRules(rulesDir, dir, nil, nil)
	require.NoError(t, err)

	assert.Empty(t, result.GrepEngine, "no grep rules ran, so no grep implementation is named")
}

// The error must be distinguishable, so a caller can tell an engine failure from
// a rule-loading failure.
func TestEngineFailureIsIdentifiable(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"internal/a.go": "package a\n\nvar _ = Sprintf\n",
	})
	rulesDir := setupRulesDir(t, dir)
	writeRule(t, rulesDir, "rule.yaml", `
id: no-sprintf
engine: grep
description: "No Sprintf"
severity: error
pattern: 'Sprintf'
scope:
  - "internal/**/*.go"
`)

	_, err := RunRules(rulesDir, dir, nil, engineclient.New("/nonexistent/verikt-engine"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrGrepEngineFailed),
		"callers need to recognise an engine failure: %v", err)
}
