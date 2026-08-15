package experiment

import (
	"testing"
	"time"

	"github.com/diktahq/verikt/internal/config"
	"github.com/stretchr/testify/assert"
)

// engineHexagonalComponents returns component config for the engine dep test.
// Uses the verikt-module testdata so the engine's module-path stripping works.
// domain/domain.go imports service — a clear hexagonal violation.
func engineHexagonalComponents() []config.Component {
	return []config.Component{
		{Name: "domain", In: []string{"domain/**"}},
		{Name: "service", In: []string{"service/**"}},
	}
}

// TestDeps_EnginePath runs dependency checks via the Rust import graph engine.
// Uses repo root as project path with verikt-module testdata so import paths can be resolved.
func TestDeps_EnginePath(t *testing.T) {
	client := newEngineClient(t)
	projectPath := engineHexagonalPath(t)
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
