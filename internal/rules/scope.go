package rules

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// skipDirs are non-hidden directories always excluded from scope expansion.
// Hidden directories (starting with ".") are skipped unconditionally.
var skipDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
}

// ExpandScope returns all files matching scope globs minus exclude globs,
// relative to projectRoot. If allowedFiles is non-nil, results are intersected
// with that set (used for --staged filtering).
func ExpandScope(scope, exclude []string, projectRoot string, allowedFiles []string) ([]string, error) {
	allowedSet := make(map[string]bool, len(allowedFiles))
	for _, f := range allowedFiles {
		allowedSet[f] = true
	}

	matchSet := map[string]bool{}

	err := filepath.WalkDir(projectRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		// Skip symlinked directories — they point outside the project boundary (INV-002).
		if d.IsDir() && d.Type()&fs.ModeSymlink != 0 {
			return filepath.SkipDir
		}

		// Skip hidden directories (starting with ".") and known non-source dirs.
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || skipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return nil
		}
		// Normalize to forward slashes for glob matching.
		rel = filepath.ToSlash(rel)

		// Check scope globs.
		matched := false
		for _, pattern := range scope {
			if matchGlob(pattern, rel) {
				matched = true
				break
			}
		}
		if !matched {
			return nil
		}

		// Check exclude globs.
		for _, pattern := range exclude {
			if matchGlob(pattern, rel) {
				return nil
			}
		}

		// If filtering by allowed files, check membership.
		if len(allowedFiles) > 0 && !allowedSet[rel] {
			return nil
		}

		matchSet[rel] = true
		return nil
	})
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(matchSet))
	for f := range matchSet {
		files = append(files, f)
	}
	sort.Strings(files)
	return files, nil
}

// matchGlob matches a file path against a glob pattern, supporting ** for
// recursive directory matching.
func matchGlob(pattern, path string) bool {
	// Handle ** patterns by splitting on /** or **/ segments.
	if strings.Contains(pattern, "**") {
		return matchDoublestar(pattern, path)
	}
	matched, _ := filepath.Match(pattern, path)
	return matched
}

// matchDoublestar handles glob patterns containing **.
func matchDoublestar(pattern, path string) bool {
	// Simple cases.
	if pattern == "**" || pattern == "**/*" {
		return true
	}

	parts := strings.SplitN(pattern, "**", 2)
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")

	// Check prefix match.
	if prefix != "" && !strings.HasPrefix(path, prefix+"/") && path != prefix {
		return false
	}

	// Strip the prefix from path for suffix matching.
	remaining := path
	if prefix != "" {
		remaining = strings.TrimPrefix(path, prefix+"/")
	}

	// If no suffix, any remaining path matches.
	if suffix == "" {
		return true
	}

	// The suffix might itself contain further path segments.
	// Try matching the suffix against every possible tail of remaining.
	segments := strings.Split(remaining, "/")
	for i := range segments {
		tail := strings.Join(segments[i:], "/")
		if matched, _ := filepath.Match(suffix, tail); matched {
			return true
		}
		// Also try matching just the filename part for patterns like "*.go".
		if !strings.Contains(suffix, "/") {
			if matched, _ := filepath.Match(suffix, segments[len(segments)-1]); matched {
				return true
			}
			return false
		}
	}

	return false
}
