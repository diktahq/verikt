package experiment

import (
	"testing"
	"time"

	"github.com/dcsg/archway/internal/checker"
	"github.com/dcsg/archway/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var metricsRules = config.FunctionRules{
	MaxLines:  5,
	MaxParams: 3,
}

// TestMetrics_GoPath runs function metric checks via the Go AST path.
func TestMetrics_GoPath(t *testing.T) {
	cfg := &config.ArchwayConfig{
		Language: "go",
		Rules:    config.RulesConfig{Functions: metricsRules},
	}
	projectPath := hexagonalProjectPath(t)

	start := time.Now()
	result, err := checker.Check(cfg, projectPath)
	duration := time.Since(start)
	require.NoError(t, err)

	t.Logf("Go AST path (%v): %d function violations", duration, len(result.FunctionViolations))
	for _, v := range result.FunctionViolations {
		t.Logf("  [%s] %s:%d — %s", v.Rule, v.File, v.Line, v.Message)
	}

	// Control: max_lines=5 must fire on the testdata (handler has functions > 5 lines).
	assert.NotEmpty(t, result.FunctionViolations, "Go path must detect function violations with max_lines=5")
}

// TestMetrics_EnginePath runs function metric checks via the Rust engine.
func TestMetrics_EnginePath(t *testing.T) {
	client := newEngineClient(t)
	projectPath := hexagonalProjectPath(t)

	start := time.Now()
	violations := checkFunctionMetrics(t, client, projectPath, metricsRules)
	duration := time.Since(start)

	t.Logf("Engine path (%v): %d function violations", duration, len(violations))
	for _, v := range violations {
		t.Logf("  [%s] %s:%d — %s", v.Rule, v.File, v.Line, v.Message)
	}

	assert.NotEmpty(t, violations, "engine must detect function violations with max_lines=5")
}

// TestMetrics_Parity compares Go AST and engine paths on the same codebase.
func TestMetrics_Parity(t *testing.T) {
	client := newEngineClient(t)
	projectPath := hexagonalProjectPath(t)

	cfg := &config.ArchwayConfig{
		Language: "go",
		Rules:    config.RulesConfig{Functions: metricsRules},
	}

	goStart := time.Now()
	goResult, err := checker.Check(cfg, projectPath)
	goDuration := time.Since(goStart)
	require.NoError(t, err)

	engineStart := time.Now()
	engineViolations := checkFunctionMetrics(t, client, projectPath, metricsRules)
	engineDuration := time.Since(engineStart)

	t.Logf("=== Metrics Parity ===")
	t.Logf("Go AST:  %d violations in %v", len(goResult.FunctionViolations), goDuration)
	t.Logf("Engine:  %d violations in %v", len(engineViolations), engineDuration)
	if goDuration > 0 && engineDuration > 0 {
		t.Logf("Speedup: %.1fx", float64(goDuration)/float64(engineDuration))
	}

	// Both must agree there are violations.
	assert.NotEmpty(t, goResult.FunctionViolations, "Go must find violations")
	assert.NotEmpty(t, engineViolations, "engine must find violations")

	// Count should be in the same ballpark — tree-sitter and go/ast may differ slightly
	// on edge cases (e.g. generated code), but should be close.
	goCt := len(goResult.FunctionViolations)
	engineCt := len(engineViolations)
	diff := goCt - engineCt
	if diff < 0 {
		diff = -diff
	}
	t.Logf("Count diff: %d (Go=%d Engine=%d)", diff, goCt, engineCt)

	// Rule names differ by implementation (Go: "max_lines", engine: "function_lines")
	// but both must detect violations across the same constraint types.
	t.Logf("Go rules fired:     %v", keys(ruleSet(goResult.FunctionViolations)))
	t.Logf("Engine rules fired: %v", keys(ruleSet(engineViolations)))
}

// TestMetrics_RealCodebase benchmarks both paths on archway's own codebase.
func TestMetrics_RealCodebase(t *testing.T) {
	client := newEngineClient(t)
	projectPath := findRepoRoot(t)

	realRules := config.FunctionRules{MaxLines: 50, MaxParams: 5}
	cfg := &config.ArchwayConfig{
		Language: "go",
		Rules:    config.RulesConfig{Functions: realRules},
	}

	goStart := time.Now()
	goResult, err := checker.Check(cfg, projectPath)
	goDuration := time.Since(goStart)
	require.NoError(t, err)

	engineStart := time.Now()
	engineViolations := checkFunctionMetrics(t, client, projectPath, realRules)
	engineDuration := time.Since(engineStart)

	t.Logf("=== Metrics: Real Codebase ===")
	t.Logf("Go AST:  %d violations in %v", len(goResult.FunctionViolations), goDuration)
	t.Logf("Engine:  %d violations in %v", len(engineViolations), engineDuration)
	if goDuration > 0 && engineDuration > 0 {
		t.Logf("Speedup: %.1fx", float64(goDuration)/float64(engineDuration))
	}
}

func ruleSet(violations []checker.Violation) map[string]bool {
	out := map[string]bool{}
	for _, v := range violations {
		out[v.Rule] = true
	}
	return out
}
