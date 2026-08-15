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
// This table is the single source of truth for detector severity.
//
// A detector ships at warning and earns error severity by demonstrating
// precision on real code. Error severity fails a build, so a false positive
// there costs the user a green pipeline and, when the finding is unfixable, a
// rewrite of code that was correct. A warning-level false positive is noise.
// Same defect, wildly different cost — so the burden of proof sits with the
// detector, not the user.
//
// Error severity today is for the structural and syntactic checks: an import
// that crosses a layer, a write to a map that was never allocated, an error
// silently discarded. Those read the AST or the import graph and can be wrong
// about intent but not about fact. The Go
// detectors it was originally mirrored from are deleted (ADR-006, ADR-011), so
// there is no second table to drift from — TestDetectorSeverityCoversEveryEngineDetector
// instead compares it against the detectors the engine actually defines, in both
// directions, and pins each severity.
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
	"nil_map_write":                 "error",
	// Textual, therefore heuristic: it reads string literals rather than a
	// resolved query, so it cannot know a match is SQL. It fired on English prose
	// in a module with no database — at error severity, which failed the build and
	// could not be fixed in the code. Warning until it has demonstrated precision
	// on real projects; raise it with severity_overrides if you want it blocking.
	"sql_concatenation":         "warning",
	"swallowed_error":           "error",
	"type_assertion_without_ok": "warning",
	"uuid_v4_as_key":            "info",
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
