package engineclient

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goDetector matches a detector literal in checker.checkAntiPatterns, capturing
// the name and the severity that follows it.
//
// Detector names contain digits (uuid_v4_as_key), so the character class must
// allow them — omitting them silently skips that detector instead of failing.
var goDetector = regexp.MustCompile(`(?s)Name:\s+"([a-z0-9_]+)",.{0,200}?Severity:\s+"([a-z]+)"`)

// TestDetectorSeverityMatchesGoDetectors asserts the engine-side severity table
// agrees with the severities the Go detectors assign.
//
// The two implementations must classify a finding identically. They did not:
// the engine stamped every anti-pattern with the requesting rule's severity
// (warning), so sql_concatenation and swallowed_error were reported as warnings
// on the engine path and errors on the Go path. Because `verikt check` gates its
// exit code on error severity, whether a SQL-injection finding failed the build
// depended on whether the embedded engine binary resolved.
func TestDetectorSeverityMatchesGoDetectors(t *testing.T) {
	checkerPath := filepath.Join("..", "checker", "antipatterns.go")
	source, err := os.ReadFile(checkerPath)
	require.NoError(t, err)

	matches := goDetector.FindAllStringSubmatch(string(source), -1)
	require.NotEmpty(t, matches, "no detectors parsed from %s — did the literal shape change?", checkerPath)

	seen := map[string]bool{}
	for _, m := range matches {
		name, severity := m[1], m[2]
		seen[name] = true

		got, ok := detectorSeverities[name]
		if !assert.True(t, ok, "detector %q has no entry in detectorSeverities", name) {
			continue
		}
		assert.Equal(t, severity, got, "severity for %q disagrees with the Go detector", name)
	}

	for name := range detectorSeverities {
		assert.True(t, seen[name], "detectorSeverities lists %q, which no Go detector produces", name)
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
