// Package pathglob matches project-relative paths against the glob patterns used
// in verikt.yaml (check.exclude, severity_overrides paths, component In lists).
//
// It exists because the same matcher was reimplemented per package and the copies
// disagreed. checker.isExcluded tested `strings.Contains(path, "gen")` for the
// pattern "gen/**", so excluding generated code also silently dropped every
// finding in internal/agent — including SQL injection — while config.matchesPaths,
// whose comment claimed to use "the same glob strategy", anchored the prefix
// correctly.
package pathglob

import (
	"path/filepath"
	"strings"
)

// Match reports whether a project-relative, slash-separated path matches pattern.
//
// Supported forms:
//
//	dir/**      the directory itself and everything beneath it
//	**/*_test.go  any path whose base name matches the tail
//	*.go        a name in the path's own directory
//
// Matching is anchored at the start of the path: "gen/**" matches "gen/a.go" but
// not "internal/gen/a.go", because a pattern that matched anywhere would silence
// findings the author never named.
func Match(pattern, path string) bool {
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")

	if pattern == "**" || pattern == "**/*" {
		return true
	}

	// A leading "**/" means "at any depth": the rest of the pattern is matched
	// against every suffix of the path. This is what makes "**/testdata/**" reach
	// a testdata directory nested anywhere, while plain "testdata/**" stays
	// anchored to the project root.
	//
	// Tested before the "/**" suffix, or "**/testdata/**" would be read as a
	// literal directory named "**".
	if tail, ok := strings.CutPrefix(pattern, "**/"); ok {
		segments := strings.Split(path, "/")
		for i := range segments {
			if Match(tail, strings.Join(segments[i:], "/")) {
				return true
			}
		}
		return false
	}

	if prefix, ok := strings.CutSuffix(pattern, "/**"); ok {
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}

	matched, _ := filepath.Match(pattern, path)
	return matched
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
