package engineclient

// detectorSeverity maps a detector name to its severity.
//
// The engine reports every anti-pattern with the severity of the single
// anti-pattern rule it was asked to run (warning), so severity cannot come from
// the wire: doing that silently downgraded sql_concatenation and swallowed_error
// from error to warning whenever the embedded engine resolved — which is the
// default path. Since `verikt check` gates its exit code on error severity, a
// SQL-injection finding stopped failing CI depending on which binary ran.
//
// These values are the same ones the Go detectors assign in
// checker.checkAntiPatterns. TestDetectorSeverityMatchesGoDetectors keeps the two
// tables in step.
var detectorSeverities = map[string]string{
	"context_background_in_handler": "warning",
	"domain_imports_adapter":        "error",
	"fat_handler":                   "warning",
	"global_mutable_state":          "warning",
	"god_package":                   "warning",
	"init_abuse":                    "warning",
	"init_side_effects":             "warning",
	"mvc_in_hexagonal":              "warning",
	"naked_goroutine":               "warning",
	"sql_concatenation":             "error",
	"swallowed_error":               "error",
	"uuid_v4_as_key":                "info",
}

// detectorSeverity returns the severity for a detector, falling back to the
// severity the engine reported for anything not in the table (a detector the
// engine gained but Go has not caught up with).
func detectorSeverity(detector, reported string) string {
	if severity, ok := detectorSeverities[detector]; ok {
		return severity
	}
	return reported
}
