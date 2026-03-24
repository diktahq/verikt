package experiment

import (
	"testing"
	"time"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetrics_MaxParams verifies max_params fires on both paths.
// hexagonal-project/adapter has handlers with >3 params.
func TestMetrics_MaxParams(t *testing.T) {
	rules := config.FunctionRules{MaxParams: 2}
	projectPath := hexagonalProjectPath(t)

	cfg := &config.VeriktConfig{
		Language: "go",
		Rules:    config.RulesConfig{Functions: rules},
	}

	goStart := time.Now()
	goResult, err := checker.Check(cfg, projectPath)
	goDuration := time.Since(goStart)
	require.NoError(t, err)

	client := newEngineClient(t)
	engineStart := time.Now()
	engineViolations := checkFunctionMetrics(t, client, projectPath, rules)
	engineDuration := time.Since(engineStart)

	t.Logf("=== max_params=2 ===")
	t.Logf("Go AST:  %d violations in %v", len(goResult.FunctionViolations), goDuration)
	t.Logf("Engine:  %d violations in %v", len(engineViolations), engineDuration)
	for _, v := range goResult.FunctionViolations {
		t.Logf("  [Go]     %s:%d — %s", v.File, v.Line, v.Message)
	}
	for _, v := range engineViolations {
		t.Logf("  [Engine] %s:%d — %s", v.File, v.Line, v.Message)
	}

	assert.NotEmpty(t, goResult.FunctionViolations, "Go path must detect max_params=2 violations")
	assert.NotEmpty(t, engineViolations, "engine must detect max_params=2 violations")
}

// TestMetrics_MaxReturnValues verifies max_return_values fires on both paths.
func TestMetrics_MaxReturnValues(t *testing.T) {
	rules := config.FunctionRules{MaxReturnValues: 1}
	projectPath := hexagonalProjectPath(t)

	cfg := &config.VeriktConfig{
		Language: "go",
		Rules:    config.RulesConfig{Functions: rules},
	}

	goStart := time.Now()
	goResult, err := checker.Check(cfg, projectPath)
	goDuration := time.Since(goStart)
	require.NoError(t, err)

	client := newEngineClient(t)
	engineStart := time.Now()
	engineViolations := checkFunctionMetrics(t, client, projectPath, rules)
	engineDuration := time.Since(engineStart)

	t.Logf("=== max_return_values=1 ===")
	t.Logf("Go AST:  %d violations in %v", len(goResult.FunctionViolations), goDuration)
	t.Logf("Engine:  %d violations in %v", len(engineViolations), engineDuration)

	assert.NotEmpty(t, goResult.FunctionViolations, "Go path must detect max_return_values=1 violations")
	assert.NotEmpty(t, engineViolations, "engine must detect max_return_values=1 violations")
}

// TestMetrics_ZeroConstraint verifies that zero-value constraints are ignored (no false positives).
func TestMetrics_ZeroConstraint(t *testing.T) {
	// All constraints at zero — should produce no violations.
	rules := config.FunctionRules{MaxLines: 0, MaxParams: 0, MaxReturnValues: 0}
	projectPath := hexagonalProjectPath(t)

	cfg := &config.VeriktConfig{
		Language: "go",
		Rules:    config.RulesConfig{Functions: rules},
	}

	goResult, err := checker.Check(cfg, projectPath)
	require.NoError(t, err)
	assert.Empty(t, goResult.FunctionViolations,
		"zero constraints must produce no function violations")

	client := newEngineClient(t)
	engineViolations := checkFunctionMetrics(t, client, projectPath, rules)
	assert.Empty(t, engineViolations,
		"engine with zero constraints must produce no function violations")
}

// TestMetrics_CombinedConstraints verifies all three constraints together.
func TestMetrics_CombinedConstraints(t *testing.T) {
	rules := config.FunctionRules{MaxLines: 5, MaxParams: 2, MaxReturnValues: 1}
	projectPath := hexagonalProjectPath(t)

	cfg := &config.VeriktConfig{
		Language: "go",
		Rules:    config.RulesConfig{Functions: rules},
	}

	goStart := time.Now()
	goResult, err := checker.Check(cfg, projectPath)
	goDuration := time.Since(goStart)
	require.NoError(t, err)

	client := newEngineClient(t)
	engineStart := time.Now()
	engineViolations := checkFunctionMetrics(t, client, projectPath, rules)
	engineDuration := time.Since(engineStart)

	t.Logf("=== Combined constraints (lines=5 params=2 returns=1) ===")
	t.Logf("Go AST:  %d violations in %v", len(goResult.FunctionViolations), goDuration)
	t.Logf("Engine:  %d violations in %v", len(engineViolations), engineDuration)

	assert.NotEmpty(t, goResult.FunctionViolations, "Go path must find combined violations")
	assert.NotEmpty(t, engineViolations, "engine must find combined violations")
}
