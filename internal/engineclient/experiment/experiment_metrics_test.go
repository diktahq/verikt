package experiment

import (
	"testing"
	"time"

	"github.com/diktahq/verikt/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var metricsRules = config.FunctionRules{
	MaxLines:  5,
	MaxParams: 3,
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

// TestMetrics_RealCodebase benchmarks both paths on verikt's own codebase.
func TestMetrics_RealCodebase(t *testing.T) {
	client := newEngineClient(t)
	projectPath := findRepoRoot(t)

	realRules := config.FunctionRules{MaxLines: 50, MaxParams: 5}
	cfg := &config.VeriktConfig{
		Language: "go",
		Rules:    config.RulesConfig{Functions: realRules},
	}

	goStart := time.Now()
	goResult, err := checkViaEngine(t, cfg, projectPath)
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
