package experiment

import (
	"testing"
	"time"

	"github.com/dcsg/archway/internal/checker"
	"github.com/dcsg/archway/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// engineHexagonalComponents returns component config for the engine dep test.
// Uses the archway-module testdata so the engine's module-path stripping works.
// domain/domain.go imports service — a clear hexagonal violation.
func engineHexagonalComponents() []config.Component {
	p := "internal/engineclient/experiment/testdata/hexagonal"
	return []config.Component{
		{Name: "domain", In: []string{p + "/domain/**"}},
		{Name: "service", In: []string{p + "/service/**"}},
	}
}

// TestDeps_GoPath runs dependency checks via the Go packages path.
// Uses the hexagonal project dir directly (go/packages operates on that module).
func TestDeps_GoPath(t *testing.T) {
	// Go packages path uses component patterns relative to the project.
	goComponents := []config.Component{
		{Name: "domain", In: []string{"domain/**"}},
		{Name: "port", In: []string{"port/**"}},
		{Name: "service", In: []string{"service/**"}, MayDependOn: []string{"domain", "port"}},
		{Name: "adapter", In: []string{"adapter/**"}, MayDependOn: []string{"domain", "port", "service"}},
	}
	cfg := &config.ArchwayConfig{Language: "go", Components: goComponents}
	projectPath := hexagonalProjectPath(t)

	start := time.Now()
	result, err := checker.Check(cfg, projectPath)
	duration := time.Since(start)
	require.NoError(t, err)

	t.Logf("Go packages path (%v): %d dependency violations", duration, len(result.DependencyViolations))
	for _, v := range result.DependencyViolations {
		t.Logf("  [%s] %s — %s", v.Rule, v.File, v.Message)
	}

	// Control: domain/bad_dep.go imports service — must be caught.
	assert.NotEmpty(t, result.DependencyViolations, "Go path must detect domain→service violation")
}

// TestDeps_EnginePath runs dependency checks via the Rust import graph engine.
// Uses repo root as project path with archway-module testdata so import paths can be resolved.
func TestDeps_EnginePath(t *testing.T) {
	client := newEngineClient(t)
	projectPath := findRepoRoot(t)
	components := engineHexagonalComponents()

	start := time.Now()
	violations := checkDependencies(t, client, projectPath, components)
	duration := time.Since(start)

	t.Logf("Engine path (%v): %d dependency violations", duration, len(violations))
	for _, v := range violations {
		t.Logf("  [%s] %s — %s", v.Rule, v.File, v.Message)
	}

	// Control: same violation must be caught by the engine.
	assert.NotEmpty(t, violations, "engine must detect domain→service violation")
}

// TestDeps_Parity compares Go packages and engine import graph on the hexagonal testdata.
func TestDeps_Parity(t *testing.T) {
	client := newEngineClient(t)

	// Go path: project = hexagonal dir, patterns relative to it.
	goComponents := []config.Component{
		{Name: "domain", In: []string{"domain/**"}},
		{Name: "port", In: []string{"port/**"}},
		{Name: "service", In: []string{"service/**"}, MayDependOn: []string{"domain", "port"}},
		{Name: "adapter", In: []string{"adapter/**"}, MayDependOn: []string{"domain", "port", "service"}},
	}
	cfg := &config.ArchwayConfig{Language: "go", Components: goComponents}

	goStart := time.Now()
	goResult, err := checker.Check(cfg, hexagonalProjectPath(t))
	goDuration := time.Since(goStart)
	require.NoError(t, err)

	engineStart := time.Now()
	engineViolations := checkDependencies(t, client, findRepoRoot(t), engineHexagonalComponents())
	engineDuration := time.Since(engineStart)

	t.Logf("=== Dep Parity ===")
	t.Logf("Go packages: %d violations in %v", len(goResult.DependencyViolations), goDuration)
	t.Logf("Engine:      %d violations in %v", len(engineViolations), engineDuration)
	if goDuration > 0 && engineDuration > 0 {
		t.Logf("Speedup: %.1fx", float64(goDuration)/float64(engineDuration))
	}

	// Both must detect the known domain→service violation.
	assert.NotEmpty(t, goResult.DependencyViolations, "Go must find dep violations")
	assert.NotEmpty(t, engineViolations, "engine must find dep violations")

	// Violated component rules should overlap.
	goRules := violationRuleSet(goResult.DependencyViolations)
	engineRules := violationRuleSet(engineViolations)
	t.Logf("Go violated rules:     %v", goRules)
	t.Logf("Engine violated rules: %v", engineRules)
}

func violationRuleSet(violations []checker.Violation) map[string]bool {
	out := map[string]bool{}
	for _, v := range violations {
		out[v.Rule] = true
	}
	return out
}
