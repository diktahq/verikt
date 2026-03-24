package experiment

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/guide"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// conformingProjectPath returns the path to the conforming-hexagonal testdata.
// This project was written by following the verikt guide — it has zero violations.
func conformingProjectPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(filename), "testdata", "conforming-hexagonal")
}

// hexagonalVeriktConfig returns the verikt config that governs both the
// violating and the conforming hexagonal projects.
func hexagonalVeriktConfig() *config.VeriktConfig {
	return &config.VeriktConfig{
		Language:     "go",
		Architecture: "hexagonal",
		Components: []config.Component{
			{Name: "domain", In: []string{"domain/**"}},
			{Name: "ports", In: []string{"port/**"}, MayDependOn: []string{"domain"}},
			{Name: "service", In: []string{"service/**"}, MayDependOn: []string{"domain", "ports"}},
			{Name: "adapter", In: []string{"adapter/**"}, MayDependOn: []string{"domain", "ports", "service"}},
		},
		Rules: config.RulesConfig{
			Functions: config.FunctionRules{
				MaxLines:        50,
				MaxParams:       4,
				MaxReturnValues: 2,
			},
		},
	}
}

// TestE2E_GuideGeneratesInstructions validates that verikt guide produces
// architecture-specific content from the declared config.
// This is the prompt the AI agent receives before writing code.
func TestE2E_GuideGeneratesInstructions(t *testing.T) {
	cfg := hexagonalVeriktConfig()
	opts := guide.GenerateOptions{
		Architecture: cfg.Architecture,
		Components:   cfg.Components,
		GuideMode:    "passive",
	}

	content := guide.BuildContent(opts)

	t.Logf("=== Generated Guide (prompt fed to AI agent) ===")
	t.Logf("%s", content)

	// The guide must describe the architecture.
	assert.Contains(t, content, "hexagonal", "guide must identify the architecture")

	// The guide must name every component.
	for _, c := range cfg.Components {
		assert.Contains(t, content, c.Name,
			"guide must reference component %q so agent knows where code belongs", c.Name)
	}

	// The guide must state dependency direction.
	assert.Contains(t, content, "domain",
		"guide must anchor the dependency direction from the domain outward")

	// The guide must list anti-patterns so the agent avoids them.
	assert.Contains(t, content, "Anti-patterns", "guide must include anti-pattern warnings")
}

// TestE2E_ViolatingProject_Check demonstrates what happens when an AI agent
// writes code WITHOUT the guide — or ignores it. This is the "bad" state.
//
// The hexagonal-project testdata simulates code written without architectural
// awareness: domain imports service, handlers have business logic, errors are
// swallowed. verikt check finds all of it.
func TestE2E_ViolatingProject_Check(t *testing.T) {
	cfg := hexagonalVeriktConfig()
	projectPath := hexagonalProjectPath(t)

	start := time.Now()
	result, err := checker.Check(cfg, projectPath)
	duration := time.Since(start)
	require.NoError(t, err)

	t.Logf("=== Without guide: violating project (%v) ===", duration)
	t.Logf("  Passed:      %v", result.Passed())
	t.Logf("  Compliance:  %.0f%%", result.Compliance()*100)
	t.Logf("  Dependencies:%d violation(s)", len(result.DependencyViolations))
	t.Logf("  Functions:   %d violation(s)", len(result.FunctionViolations))
	t.Logf("  Anti-patterns:%d violation(s)", len(result.AntiPatternViolations))

	for _, v := range result.DependencyViolations {
		t.Logf("    [dep]  %s — %s", v.File, v.Message)
	}
	for _, v := range result.FunctionViolations {
		t.Logf("    [fn]   %s:%d — %s", v.File, v.Line, v.Message)
	}
	for _, ap := range result.AntiPatternViolations {
		t.Logf("    [ap]   %s:%d — %s", ap.File, ap.Line, ap.Message)
	}

	// Control: this project must fail.
	assert.False(t, result.Passed(), "violating project must not pass verikt check")
	assert.NotEmpty(t, result.DependencyViolations, "must catch layer boundary violations")
	assert.NotEmpty(t, result.AntiPatternViolations, "must catch code anti-patterns")
}

// TestE2E_ConformingProject_Check demonstrates what happens when an AI agent
// writes code following the verikt guide — the "good" state.
//
// The conforming-hexagonal testdata simulates code written by an agent that
// received the guide as context: clean layer boundaries, no anti-patterns,
// functions within size limits, errors propagated correctly.
//
// verikt check must find zero violations. This is the negative result —
// and it is as important as the positive one. A tool that cries wolf gets ignored.
func TestE2E_ConformingProject_Check(t *testing.T) {
	cfg := hexagonalVeriktConfig()
	projectPath := conformingProjectPath(t)

	start := time.Now()
	result, err := checker.Check(cfg, projectPath)
	duration := time.Since(start)
	require.NoError(t, err)

	t.Logf("=== With guide: conforming project (%v) ===", duration)
	t.Logf("  Passed:      %v", result.Passed())
	t.Logf("  Compliance:  %.0f%%", result.Compliance()*100)
	t.Logf("  Dependencies:%d violation(s)", len(result.DependencyViolations))
	t.Logf("  Functions:   %d violation(s)", len(result.FunctionViolations))
	t.Logf("  Anti-patterns:%d violation(s)", len(result.AntiPatternViolations))

	// The critical assertion: zero violations on conforming code.
	assert.True(t, result.Passed(), "conforming project must pass verikt check")
	assert.Empty(t, result.DependencyViolations, "no layer boundary violations")
	assert.Empty(t, result.FunctionViolations, "no function metric violations")
	assert.Empty(t, result.AntiPatternViolations, "no anti-pattern violations")
}

// TestE2E_GuideToEnforce_Contrast runs both projects under the same config and
// logs the side-by-side contrast — the full Guide→Enforce loop.
//
// This is the blog post moment: same rules, same enforcement, two codebases.
// One was written with architectural awareness. One was not.
func TestE2E_GuideToEnforce_Contrast(t *testing.T) {
	cfg := hexagonalVeriktConfig()

	// Generate the guide — this is what the agent receives.
	opts := guide.GenerateOptions{
		Architecture: cfg.Architecture,
		Components:   cfg.Components,
		GuideMode:    "passive",
	}
	guideContent := guide.BuildContent(opts)

	lineCount := len(strings.Split(guideContent, "\n"))
	t.Logf("Guide generated: %d lines of architecture instructions", lineCount)

	// Without guide: violating project.
	violatingStart := time.Now()
	violatingResult, err := checker.Check(cfg, hexagonalProjectPath(t))
	violatingDuration := time.Since(violatingStart)
	require.NoError(t, err)

	// With guide: conforming project.
	conformingStart := time.Now()
	conformingResult, err := checker.Check(cfg, conformingProjectPath(t))
	conformingDuration := time.Since(conformingStart)
	require.NoError(t, err)

	t.Logf("")
	t.Logf("=== Guide → Enforce: Side-by-Side Contrast ===")
	t.Logf("")
	t.Logf("  %-28s  %-12s  %-12s", "Metric", "Without Guide", "With Guide")
	t.Logf("  %-28s  %-12s  %-12s", strings.Repeat("-", 28), strings.Repeat("-", 12), strings.Repeat("-", 12))
	t.Logf("  %-28s  %-12v  %-12v", "Passed", violatingResult.Passed(), conformingResult.Passed())
	t.Logf("  %-28s  %-12s  %-12s",
		"Compliance",
		formatPct(violatingResult.Compliance()),
		formatPct(conformingResult.Compliance()),
	)
	t.Logf("  %-28s  %-12d  %-12d", "Dependency violations",
		len(violatingResult.DependencyViolations), len(conformingResult.DependencyViolations))
	t.Logf("  %-28s  %-12d  %-12d", "Function violations",
		len(violatingResult.FunctionViolations), len(conformingResult.FunctionViolations))
	t.Logf("  %-28s  %-12d  %-12d", "Anti-pattern violations",
		len(violatingResult.AntiPatternViolations), len(conformingResult.AntiPatternViolations))
	t.Logf("  %-28s  %-12v  %-12v", "Analysis time",
		violatingDuration.Round(time.Millisecond),
		conformingDuration.Round(time.Millisecond),
	)
	t.Logf("")

	// The negative result is the proof: guide works.
	assert.False(t, violatingResult.Passed(), "without guide: must fail")
	assert.True(t, conformingResult.Passed(), "with guide: must pass — zero violations")
	assert.Equal(t, 1.0, conformingResult.Compliance(), "conforming project must be 100%% compliant")
}

func formatPct(f float64) string {
	return fmt.Sprintf("%.0f%%", f*100)
}
