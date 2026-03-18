package experiment

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dcsg/archway/internal/checker"
	"github.com/dcsg/archway/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// controlAntiPatterns lists detectors that must fire on the hexagonal testdata.
var controlAntiPatterns = []string{
	"global_mutable_state",          // domain/errors.go — var GlobalCache = map[...]...
	"swallowed_error",               // service/order.go — if err != nil {}
	"naked_goroutine",               // adapter/httphandler/handler.go — go func() { ... }()
	"context_background_in_handler", // adapter/httphandler/handler.go — context.Background()
	"sql_concatenation",             // adapter/mysqlrepo/repo.go — "SELECT..." + id
}

// TestAntiPatterns_GoPath runs anti-pattern detection via the Go AST path.
func TestAntiPatterns_GoPath(t *testing.T) {
	cfg := &config.ArchwayConfig{Language: "go"}
	projectPath := hexagonalProjectPath(t)

	start := time.Now()
	result, err := checker.Check(cfg, projectPath)
	duration := time.Since(start)
	require.NoError(t, err)

	found := detectorSet(result)
	t.Logf("Go AST path (%v): %d anti-patterns found", duration, len(result.AntiPatternViolations))
	logAntiPatterns(t, result)

	for _, name := range controlAntiPatterns {
		assert.True(t, found[name], "Go path must detect %q", name)
	}
}

// TestAntiPatterns_EnginePath runs anti-pattern detection via the Rust engine.
func TestAntiPatterns_EnginePath(t *testing.T) {
	client := newEngineClient(t)
	projectPath := hexagonalProjectPath(t)

	start := time.Now()
	findings, err := client.CheckAntiPatterns(projectPath, nil)
	duration := time.Since(start)
	require.NoError(t, err)

	found := map[string]bool{}
	for _, f := range findings {
		found[f.Name] = true
	}
	t.Logf("Engine path (%v): %d anti-patterns found", duration, len(findings))
	for _, f := range findings {
		t.Logf("  [%s] %s:%d — %s", f.Name, filepath.Base(f.File), f.Line, f.Message)
	}

	for _, name := range controlAntiPatterns {
		assert.True(t, found[name], "engine must detect %q", name)
	}
}

// TestAntiPatterns_Parity compares Go and engine paths detector-by-detector.
func TestAntiPatterns_Parity(t *testing.T) {
	client := newEngineClient(t)
	cfg := &config.ArchwayConfig{Language: "go"}
	projectPath := hexagonalProjectPath(t)

	goStart := time.Now()
	goResult, err := checker.Check(cfg, projectPath)
	goDuration := time.Since(goStart)
	require.NoError(t, err)
	goFound := detectorSet(goResult)

	engineStart := time.Now()
	engineFindings, err := client.CheckAntiPatterns(projectPath, nil)
	engineDuration := time.Since(engineStart)
	require.NoError(t, err)
	engineFound := map[string]bool{}
	for _, f := range engineFindings {
		engineFound[f.Name] = true
	}

	t.Logf("=== Anti-pattern Parity ===")
	t.Logf("Go AST:  %d findings in %v", len(goResult.AntiPatternViolations), goDuration)
	t.Logf("Engine:  %d findings in %v", len(engineFindings), engineDuration)
	if goDuration > 0 && engineDuration > 0 {
		t.Logf("Speedup: %.1fx", float64(goDuration)/float64(engineDuration))
	}

	t.Logf("Detectors fired — Go: %v", keys(goFound))
	t.Logf("Detectors fired — Engine: %v", keys(engineFound))

	// Control: all required detectors must fire on both paths.
	for _, name := range controlAntiPatterns {
		assert.True(t, goFound[name], "Go must fire %q", name)
		assert.True(t, engineFound[name], "engine must fire %q", name)
	}
}

func detectorSet(result *checker.CheckResult) map[string]bool {
	found := map[string]bool{}
	for _, ap := range result.AntiPatternViolations {
		found[ap.Name] = true
	}
	return found
}

func logAntiPatterns(t *testing.T, result *checker.CheckResult) {
	t.Helper()
	for _, ap := range result.AntiPatternViolations {
		t.Logf("  [%s] %s:%d — %s", ap.Name, filepath.Base(ap.File), ap.Line, ap.Message)
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
