package engineclient

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rustDetector matches a detector literal in the engine's antipatterns.rs.
var rustDetector = regexp.MustCompile(`detector:\s+"([a-z0-9_]+)"`)

// TestDetectorSeverityCoversEveryEngineDetector asserts the severity table and the
// engine's detector set agree.
//
// The table is the single source of truth for severity: the engine stamps every
// anti-pattern with the requesting rule's severity (warning), so a detector missing
// from the table is silently downgraded — which is how sql_concatenation stopped
// failing builds.
//
// It used to be validated against the Go detectors. Those were deleted with the Go
// analysis path (ADR-006, ADR-011), so the engine source is now the reference.
func TestDetectorSeverityCoversEveryEngineDetector(t *testing.T) {
	enginePath := filepath.Join("..", "..", "engine", "crates", "engine-bin", "src", "antipatterns.rs")
	source, err := os.ReadFile(enginePath)
	if err != nil {
		t.Skipf("engine source not available: %v", err)
	}

	engineDetectors := map[string]bool{}
	for _, m := range rustDetector.FindAllStringSubmatch(string(source), -1) {
		engineDetectors[m[1]] = true
	}
	require.NotEmpty(t, engineDetectors, "no detectors parsed from %s — did the literal shape change?", enginePath)

	for name := range engineDetectors {
		assert.Contains(t, detectorSeverities, name,
			"engine detector %q has no severity entry — it would be reported as a warning regardless of its real severity", name)
	}
	for name := range detectorSeverities {
		assert.True(t, engineDetectors[name],
			"detectorSeverities lists %q, which the engine never emits", name)
	}
}

// The severities that gate CI must be preserved exactly: these are the findings
// that should fail a build.
func TestDetectorSeverity_ErrorLevelFindingsAreNotDowngraded(t *testing.T) {
	for _, detector := range []string{"sql_concatenation", "swallowed_error", "domain_imports_adapter"} {
		assert.Equal(t, "error", detectorSeverity(detector, "warning"),
			"%s must stay error-severity even though the engine reports warning", detector)
	}
}

// A detector the engine gained before Go knows about it keeps the reported
// severity rather than being dropped or mislabelled.
func TestDetectorSeverity_UnknownDetectorFallsBackToReported(t *testing.T) {
	assert.Equal(t, "warning", detectorSeverity("brand_new_detector", "warning"))
}
