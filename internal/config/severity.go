package config

import (
	"path/filepath"
	"strings"
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
// Uses the same glob strategy as checker.isExcluded: "/**" suffix triggers
// prefix containment, everything else uses filepath.Match.
func matchesPaths(filePath string, paths []string) bool {
	if len(paths) == 0 {
		return true
	}
	for _, pattern := range paths {
		if prefix, ok := strings.CutSuffix(pattern, "/**"); ok {
			if strings.HasPrefix(filePath, prefix+"/") || filePath == prefix {
				return true
			}
			continue
		}
		if ok, _ := filepath.Match(pattern, filePath); ok {
			return true
		}
		// Also try matching the base name for patterns like **/*_test.go.
		if tail, ok := strings.CutPrefix(pattern, "**/"); ok {
			if matched, _ := filepath.Match(tail, filepath.Base(filePath)); matched {
				return true
			}
		}
	}
	return false
}
