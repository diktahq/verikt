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
	// Not a skip. This package is internal/, so these tests only ever run inside
	// the repository, where the engine source is always present — and a guard
	// that skips itself when it cannot find its reference is the same silent-pass
	// failure the severity table exists to prevent.
	require.NoError(t, err, "engine source is required to check detector parity")

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

// Every detector's severity is pinned by hand.
//
// Covering the *names* in both directions says nothing about the severities, and
// severity is the whole reason this table exists: the engine reports every
// finding with the requesting rule's severity, so the table is the only thing
// deciding what fails a build. Three of fourteen were pinned, which left eleven
// free to be changed without a single test noticing — the exact regression that
// downgraded sql_concatenation from error to warning.
//
// Changing a value here is a deliberate act with a diff, which is the point.
func TestEveryDetectorSeverityIsPinned(t *testing.T) {
	want := map[string]string{
		"context_background_in_handler": "warning",
		"domain_imports_adapter":        "error",
		"fat_handler":                   "warning",
		"global_mutable_state":          "warning",
		"god_package":                   "warning",
		"init_abuse":                    "warning",
		"init_side_effects":             "warning",
		"mvc_in_hexagonal":              "warning",
		"naked_goroutine":               "warning",
		"nil_map_write":                 "error",
		"sql_concatenation":             "error",
		"swallowed_error":               "error",
		"type_assertion_without_ok":     "warning",
		"uuid_v4_as_key":                "info",
	}

	// The engine reports "warning" for everything, so passing it here proves the
	// table is what decides, not the wire.
	for detector, severity := range want {
		assert.Equal(t, severity, detectorSeverity(detector, "warning"),
			"%s resolved to the wrong severity", detector)
	}

	assert.Len(t, detectorSeverities, len(want),
		"a detector was added or removed without pinning its severity here")
}

// The severities that gate CI, called out separately: these are the findings
// that must fail a build.
func TestDetectorSeverity_ErrorLevelFindingsAreNotDowngraded(t *testing.T) {
	for _, detector := range []string{"sql_concatenation", "swallowed_error", "domain_imports_adapter", "nil_map_write"} {
		assert.Equal(t, "error", detectorSeverity(detector, "warning"),
			"%s must stay error-severity even though the engine reports warning", detector)
	}
}

// A detector the engine gained before Go knows about it keeps the reported
// severity rather than being dropped or mislabelled.
func TestDetectorSeverity_UnknownDetectorFallsBackToReported(t *testing.T) {
	assert.Equal(t, "warning", detectorSeverity("brand_new_detector", "warning"))
}
