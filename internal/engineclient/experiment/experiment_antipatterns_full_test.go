package experiment

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/dcsg/archway/internal/checker"
	"github.com/dcsg/archway/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// antipatternFullProjectPath returns the testdata/antipatterns-full project path.
func antipatternFullProjectPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(filename), "testdata", "antipatterns-full")
}

// fullDetectors lists every anti-pattern that must fire on antipatterns-full.
var fullDetectors = []struct {
	name   string
	subpkg string // sub-package that triggers it
	goOnly bool   // true if Go AST only (engine may skip)
}{
	{name: "fat_handler", subpkg: "handler"},
	{name: "init_abuse", subpkg: "initabuse"},
	{name: "init_side_effects", subpkg: "initside"},
	{name: "uuid_v4_as_key", subpkg: "uuidkey"},
}

// TestAntiPatternsFull_GoPath validates that all remaining detectors fire via the Go AST path.
func TestAntiPatternsFull_GoPath(t *testing.T) {
	cfg := &config.ArchwayConfig{Language: "go"}
	projectPath := antipatternFullProjectPath(t)

	start := time.Now()
	result, err := checker.Check(cfg, projectPath)
	duration := time.Since(start)
	require.NoError(t, err)

	found := detectorSet(result)
	t.Logf("Go AST full (%v): %d anti-patterns found", duration, len(result.AntiPatternViolations))
	logAntiPatterns(t, result)

	for _, d := range fullDetectors {
		assert.True(t, found[d.name], "Go path must detect %q (triggered by %s/)", d.name, d.subpkg)
	}
}

// TestAntiPatternsFull_EnginePath validates the same detectors via the Rust engine.
func TestAntiPatternsFull_EnginePath(t *testing.T) {
	client := newEngineClient(t)
	projectPath := antipatternFullProjectPath(t)

	start := time.Now()
	findings, err := client.CheckAntiPatterns(projectPath, nil)
	duration := time.Since(start)
	require.NoError(t, err)

	found := map[string]bool{}
	for _, f := range findings {
		found[f.Name] = true
	}
	t.Logf("Engine full (%v): %d anti-patterns found", duration, len(findings))
	for _, f := range findings {
		t.Logf("  [%s] %s:%d — %s", f.Name, filepath.Base(f.File), f.Line, f.Message)
	}

	for _, d := range fullDetectors {
		if d.goOnly {
			continue
		}
		assert.True(t, found[d.name], "engine must detect %q (triggered by %s/)", d.name, d.subpkg)
	}
}

// TestAntiPatternsFull_FalsePositives verifies the clean/ package produces zero findings.
func TestAntiPatternsFull_FalsePositives(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	cleanPath := filepath.Join(filepath.Dir(filename), "testdata", "antipatterns-full", "clean")

	// Go path.
	cfg := &config.ArchwayConfig{Language: "go"}
	goResult, err := checker.Check(cfg, cleanPath)
	require.NoError(t, err)
	assert.Empty(t, goResult.AntiPatternViolations,
		"Go path: clean/ must produce zero anti-pattern findings")

	// Engine path.
	client := newEngineClient(t)
	findings, err := client.CheckAntiPatterns(cleanPath, nil)
	require.NoError(t, err)
	assert.Empty(t, findings, "engine: clean/ must produce zero anti-pattern findings")
}

// TestAntiPatternsFull_GodPackage confirms god_package fires (cross-package, needs full project).
func TestAntiPatternsFull_GodPackage(t *testing.T) {
	cfg := &config.ArchwayConfig{Language: "go"}
	projectPath := antipatternFullProjectPath(t)

	result, err := checker.Check(cfg, projectPath)
	require.NoError(t, err)

	found := detectorSet(result)
	assert.True(t, found["god_package"],
		"Go path must detect god_package (godpkg/ has 42 exported functions)")
}
