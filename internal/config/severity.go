package config

import (
	"fmt"

	"github.com/diktahq/verikt/internal/pathglob"
)

// ResolveSeverity returns the effective severity for a violation given its override key and file path.
// key matches violation.Rule, antipattern.Name, or ruleViolation.RuleID.
// filePath is the file where the violation was found (relative path).
// defaultSeverity is returned when no override matches.
// First match wins; an empty Paths slice is a catch-all.
func ResolveSeverity(overrides SeverityOverrides, key, filePath, defaultSeverity string) string {
	entries, ok := overrides[key]
	if !ok {
		return defaultSeverity
	}
	for _, entry := range entries {
		if matchesPaths(filePath, entry.Paths) {
			return entry.Severity
		}
	}
	return defaultSeverity
}

// ResolveReason returns the reason string for the matched override entry.
// Returns "" if no override matched.
func ResolveReason(overrides SeverityOverrides, key, filePath string) string {
	entries, ok := overrides[key]
	if !ok {
		return ""
	}
	for _, entry := range entries {
		if matchesPaths(filePath, entry.Paths) {
			return entry.Reason
		}
	}
	return ""
}

// MapSeverity converts a config severity value to the checker's internal severity string.
// "must" → "error", "should" → "warning", "ignore" → "ignore".
// Unrecognised values are returned as-is.
func MapSeverity(configSeverity string) string {
	switch configSeverity {
	case "must":
		return "error"
	case "should":
		return "warning"
	case "ignore":
		return "ignore"
	default:
		return configSeverity
	}
}

// matchesPaths reports whether filePath matches any glob in paths.
// An empty paths slice is a catch-all and always matches.
//
// The glob semantics live in one place (internal/pathglob). This function used
// to carry its own copy alongside checker.isExcluded's, and the comment claiming
// the two used "the same glob strategy" had stopped being true: one anchored the
// prefix, the other matched anywhere in the path.
func matchesPaths(filePath string, paths []string) bool {
	if len(paths) == 0 {
		return true
	}
	return pathglob.MatchAny(filePath, paths)
}

// Thresholds for check.fail_on and --strict: the lowest finding severity that
// fails `verikt check`.
const (
	FailOnError   = "error"   // the default: only error-severity findings fail
	FailOnWarning = "warning" // --strict, or check.fail_on: warning
)

// ValidateFailOn rejects check.fail_on values other than "", "error" and
// "warning". The severity_overrides vocabulary (must, should) does not apply:
// fail_on names a finding severity, not an override.
func ValidateFailOn(failOn string) error {
	switch failOn {
	case "", FailOnError, FailOnWarning:
		return nil
	default:
		return fmt.Errorf("check.fail_on %q is invalid (must be one of: error, warning)", failOn)
	}
}

// NormalizeSeverity maps a finding severity, after severity_overrides have been
// applied, to "error", "warning", "info" or "ignore". An empty or unrecognised
// severity becomes "error": a finding whose severity was never set keeps failing
// the check rather than passing silently.
func NormalizeSeverity(severity string) string {
	switch severity {
	case "error", "warning", "info", "ignore":
		return severity
	default:
		return "error"
	}
}

// SeverityFails is the single pass/fail rule for every finding `verikt check`
// reports — built-in detectors, anti-patterns, proxy rules and decision gates.
// It reports whether a finding of the given severity fails the run at threshold
// failOn (FailOnError or FailOnWarning; empty means FailOnError).
//
//	error, empty, unknown → fail
//	warning               → fail only at FailOnWarning
//	info, ignore          → never fail
//
// Warnings never fail by default because detectors ship at warning until they
// have shown their precision on real repositories (INV-005); a warning that
// blocked would make that distinction meaningless.
func SeverityFails(severity, failOn string) bool {
	switch NormalizeSeverity(severity) {
	case "error":
		return true
	case "warning":
		return failOn == FailOnWarning
	default:
		return false
	}
}
