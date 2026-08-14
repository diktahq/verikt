package experiment

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/diktahq/verikt/internal/checker"
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

func detectorSet(result *checker.CheckResult) map[string]bool {
	found := map[string]bool{}
	for _, ap := range result.AntiPatternViolations {
		found[ap.Name] = true
	}
	return found
}
