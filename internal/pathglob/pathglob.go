// Package pathglob matches project-relative paths against the glob patterns used
// in verikt.yaml (check.exclude, severity_overrides paths) and in proxy rule
// scopes.
//
// It exists because the same matcher was reimplemented per package and the copies
// disagreed. checker.isExcluded tested `strings.Contains(path, "gen")` for the
// pattern "gen/**", so excluding generated code also silently dropped every
// finding in internal/agent — including SQL injection — while config.matchesPaths,
// whose comment claimed to use "the same glob strategy", anchored the prefix
// correctly, and internal/rules had a third implementation with yet another set of
// behaviours. The same pattern string written in two places meant two things.
package pathglob

import (
	"path/filepath"
	"strings"
)

// Match reports whether a project-relative, slash-separated path matches pattern.
//
// Semantics, matching what globset does on the Rust side:
//
//	"*"       any run of characters within one path segment
//	"?"       any single character within one path segment
//	"**"      any number of segments, including none
//	"dir/**"  the directory itself and everything beneath it
//	"dir/"    same as "dir/**" — a trailing slash means a directory
//	"dir"     a bare name with no metacharacters also matches the tree below it
//
// Matching is anchored at the start of the path: "gen/**" matches "gen/a.go" but
// not "internal/gen/a.go", because a pattern that matched anywhere would silence
// findings the author never named. Use a leading "**/" for "at any depth".
func Match(pattern, path string) bool {
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")

	// A trailing slash, or a bare name carrying no metacharacters, names a
	// directory and so covers the tree below it. Both are how people write
	// directory patterns by habit, and both previously reached filepath.Match and
	// matched nothing.
	if trimmed, ok := strings.CutSuffix(pattern, "/"); ok {
		pattern = trimmed + "/**"
	} else if !strings.ContainsAny(pattern, "*?[") && !strings.Contains(pattern, "/") {
		if path == pattern || strings.HasPrefix(path, pattern+"/") {
			return true
		}
		return false
	}

	return matchSegments(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

// matchSegments matches pattern segments against path segments, where a "**"
// segment consumes any number of path segments including none.
//
// This replaced a chain of special cases keyed on where the "**" appeared, which
// handled a leading "**/" and a trailing "/**" and silently matched nothing for
// every other position — so "internal/**/testdata/**" and "src/**/*_test.go",
// the forms people reach for first, did nothing at all.
func matchSegments(pattern, path []string) bool {
	// Both exhausted: a match. Pattern exhausted with path left over: not a
	// match, since anchoring is at both ends unless "**" says otherwise.
	if len(pattern) == 0 {
		return len(path) == 0
	}

	if pattern[0] == "**" {
		// Trailing "**" absorbs whatever remains, including nothing.
		if len(pattern) == 1 {
			return true
		}
		// Otherwise try consuming 0, 1, 2 … path segments.
		for i := 0; i <= len(path); i++ {
			if matchSegments(pattern[1:], path[i:]) {
				return true
			}
		}
		return false
	}

	if len(path) == 0 {
		return false
	}

	matched, err := filepath.Match(pattern[0], path[0])
	if err != nil || !matched {
		return false
	}
	return matchSegments(pattern[1:], path[1:])
}

// MatchAny reports whether path matches any of the patterns. An empty pattern
// list matches nothing — callers that treat "no patterns" as a catch-all must say
// so themselves, because the two readings are not interchangeable.
func MatchAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if Match(pattern, path) {
			return true
		}
	}
	return false
}
