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

func logCheckResult(t *testing.T, label string, result *checker.CheckResult, duration time.Duration) {
	t.Helper()
	t.Logf("=== %s (%v) ===", label, duration)
	t.Logf("  Structure:    %d violations", len(result.StructureViolations))
	t.Logf("  Dependencies: %d violations", len(result.DependencyViolations))
	t.Logf("  Functions:    %d violations", len(result.FunctionViolations))
	t.Logf("  Anti-patterns:%d violations", len(result.AntiPatternViolations))
	t.Logf("  Passed: %v, Compliance: %.0f%%", result.Passed(), result.Compliance()*100)
}
