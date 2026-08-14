package checker

// applyExcludes drops findings whose file matches one of the check.exclude globs.
//
// The field is documented as paths the checker ignores entirely, but it was only
// consulted by orphan-package detection: anti-patterns and function metrics were
// still reported for generated code, vendored trees and test fixtures. Fixtures
// are the clearest case — they contain the anti-patterns the detectors look for on
// purpose, and the Go toolchain excludes testdata from builds, so the Go analysis
// path never sees them while the Rust engine does.
//
// Metrics are recalculated afterwards so compliance figures match the findings
// actually reported.
func applyExcludes(result *CheckResult, excludes []string) {
	if result == nil || len(excludes) == 0 {
		return
	}

	result.DependencyViolations = keepIncluded(result.DependencyViolations, excludes)
	result.StructureViolations = keepIncluded(result.StructureViolations, excludes)
	result.FunctionViolations = keepIncluded(result.FunctionViolations, excludes)
	result.NamingViolations = keepIncluded(result.NamingViolations, excludes)

	kept := make([]AntiPattern, 0, len(result.AntiPatternViolations))
	for _, ap := range result.AntiPatternViolations {
		if !isExcluded(ap.File, excludes) {
			kept = append(kept, ap)
		}
	}
	result.AntiPatternViolations = kept

	// Both callers run computeMetrics immediately after this, and RulesChecked is
	// still 0 at this point, so this recalculation is currently a no-op. It is
	// kept because applyExcludes must leave a consistent result for any caller,
	// not only the two that happen to recompute afterwards — a filter that
	// removes findings and leaves the counts describing the unfiltered set is the
	// kind of quiet inconsistency this package exists to catch.
	result.RecalculateMetrics()
}

// keepIncluded returns the violations whose file is not excluded.
func keepIncluded(violations []Violation, excludes []string) []Violation {
	kept := make([]Violation, 0, len(violations))
	for _, v := range violations {
		if !isExcluded(v.File, excludes) {
			kept = append(kept, v)
		}
	}
	return kept
}
