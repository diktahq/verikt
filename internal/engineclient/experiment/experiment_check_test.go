package experiment

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hexagonalConfig returns the verikt config for the hexagonal testdata project.
func hexagonalConfig() *config.VeriktConfig {
	return &config.VeriktConfig{
		Language:     "go",
		Architecture: "hexagonal",
		Components: []config.Component{
			{Name: "domain", In: []string{"domain/**"}},
			{Name: "port", In: []string{"port/**"}},
			{Name: "service", In: []string{"service/**"}, MayDependOn: []string{"domain", "port"}},
			{Name: "adapter", In: []string{"adapter/**"}, MayDependOn: []string{"domain", "port", "service"}},
		},
		Rules: config.RulesConfig{
			Structure: config.StructureConfig{
				RequiredDirs:  []string{"domain/", "port/", "service/", "adapter/"},
				ForbiddenDirs: []string{"models/", "controllers/"},
			},
			Functions: config.FunctionRules{
				MaxLines:        5,
				MaxParams:       3,
				MaxReturnValues: 2,
			},
		},
	}
}

func hexagonalProjectPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(checkerTestdataDir(t), "hexagonal-project")
}

// TestFullCheck_GoPath runs verikt check using the Go-native analysis path (no engine).
func TestFullCheck_GoPath(t *testing.T) {
	cfg := hexagonalConfig()
	projectPath := hexagonalProjectPath(t)

	start := time.Now()
	result, err := checker.Check(cfg, projectPath)
	duration := time.Since(start)

	require.NoError(t, err)
	logCheckResult(t, "Go path", result, duration)

	// Control: known violations that must be present.
	assert.NotEmpty(t, result.AntiPatternViolations, "Go path must detect anti-patterns")
	assert.NotEmpty(t, result.FunctionViolations, "Go path must detect function metric violations")
	assert.False(t, result.Passed(), "hexagonal-project has known violations")
}

// TestFullCheck_EnginePath runs verikt check using the Rust engine for all subsystems.
func TestFullCheck_EnginePath(t *testing.T) {
	client := newEngineClient(t)
	cfg := hexagonalConfig()
	projectPath := hexagonalProjectPath(t)

	apClient := &apAdapter{client}
	depClient := &depAdapter{client}
	metricClient := &metricAdapter{client}

	start := time.Now()
	result, err := checker.CheckWithEngine(cfg, projectPath, apClient, depClient, metricClient)
	duration := time.Since(start)

	require.NoError(t, err)
	logCheckResult(t, "Engine path", result, duration)

	// Control: same invariants as Go path.
	assert.NotEmpty(t, result.AntiPatternViolations, "engine path must detect anti-patterns")
	assert.NotEmpty(t, result.FunctionViolations, "engine path must detect function metric violations")
	assert.False(t, result.Passed(), "hexagonal-project has known violations")
}

// TestFullCheck_Parity runs both paths and asserts they agree on the same violation categories.
func TestFullCheck_Parity(t *testing.T) {
	client := newEngineClient(t)
	cfg := hexagonalConfig()
	projectPath := hexagonalProjectPath(t)

	goStart := time.Now()
	goResult, err := checker.Check(cfg, projectPath)
	goDuration := time.Since(goStart)
	require.NoError(t, err)

	apClient := &apAdapter{client}
	depClient := &depAdapter{client}
	metricClient := &metricAdapter{client}

	engineStart := time.Now()
	engineResult, err := checker.CheckWithEngine(cfg, projectPath, apClient, depClient, metricClient)
	engineDuration := time.Since(engineStart)
	require.NoError(t, err)

	t.Logf("=== Full Check Parity ===")
	t.Logf("Go path:     %v — ap=%d fn=%d dep=%d struct=%d",
		goDuration, len(goResult.AntiPatternViolations), len(goResult.FunctionViolations),
		len(goResult.DependencyViolations), len(goResult.StructureViolations))
	t.Logf("Engine path: %v — ap=%d fn=%d dep=%d struct=%d",
		engineDuration, len(engineResult.AntiPatternViolations), len(engineResult.FunctionViolations),
		len(engineResult.DependencyViolations), len(engineResult.StructureViolations))

	if goDuration > 0 && engineDuration > 0 {
		t.Logf("Speedup: %.1fx", float64(goDuration)/float64(engineDuration))
	}

	// Structure is Go-only in both paths — must match exactly.
	assert.Equal(t, len(goResult.StructureViolations), len(engineResult.StructureViolations),
		"structure violation count must match")

	// Both paths must agree on whether the project passes.
	assert.Equal(t, goResult.Passed(), engineResult.Passed(),
		"Passed() must agree between Go and engine paths")

	// Anti-patterns: both must fire, engine may find more (tree-sitter is more precise).
	assert.NotEmpty(t, goResult.AntiPatternViolations, "Go path must find anti-patterns")
	assert.NotEmpty(t, engineResult.AntiPatternViolations, "engine path must find anti-patterns")

	// Function metrics: both must fire with max_lines=5.
	assert.NotEmpty(t, goResult.FunctionViolations, "Go path must find function violations")
	assert.NotEmpty(t, engineResult.FunctionViolations, "engine path must find function violations")
}

func logCheckResult(t *testing.T, label string, result *checker.CheckResult, duration time.Duration) {
	t.Helper()
	t.Logf("=== %s (%v) ===", label, duration)
	t.Logf("  Structure:    %d violations", len(result.StructureViolations))
	t.Logf("  Dependencies: %d violations", len(result.DependencyViolations))
	t.Logf("  Functions:    %d violations", len(result.FunctionViolations))
	t.Logf("  Anti-patterns:%d violations", len(result.AntiPatternViolations))
	t.Logf("  Passed: %v, Compliance: %.0f%%", result.Passed(), result.Compliance()*100)
}
