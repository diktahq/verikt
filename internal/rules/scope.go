package rules

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/verikt/internal/pathglob"
)

// skipDirs are non-hidden directories always excluded from scope expansion.
// Hidden directories (starting with ".") and `_`-prefixed directories are skipped
// unconditionally.
//
// This list must match the engine's grep walker (engine/crates/engine-bin/src/grep.rs).
// It held only vendor and node_modules while the engine also skipped testdata,
// target and `_`-prefixed directories, so the two implementations reported
// different findings for identical input — and which one ran depended on whether
// the embedded engine resolved, which is invisible to the user.
// TestGrepFindingParityBetweenGoAndEngine fails if they drift apart again.
var skipDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
	"target":       true,
	"testdata":     true,
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

		// A symlink of any kind points outside the project boundary and is never
		// project-local code (INV-002).
		//
		// The guard was `d.IsDir() && d.Type()&fs.ModeSymlink != 0`, which never
		// fires: WalkDir does not follow symlinks, so it reports one from Lstat
		// and IsDir() is false even for a link to a directory. Linked directories
		// were therefore skipped by accident — WalkDir simply never descended —
		// while linked *files* fell straight through and were grepped. Testing
		// the type alone covers both, deliberately.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		// Skip hidden directories (starting with ".") and known non-source dirs.
		if d.IsDir() {
			// The walk root itself is exempt: a projectRoot of "." has
			// d.Name() == "." and would otherwise skip the entire tree,
			// silently matching 0 files for every rule.
			if path == projectRoot {
				return nil
			}
			name := d.Name()
			// `_`-prefixed directories are ignored by the Go toolchain, so code
			// there is not part of the build and should not be part of the check.
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || skipDirs[name] {
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
			if pathglob.Match(pattern, rel) {
				matched = true
				break
			}
		}
		if !matched {
			return nil
		}

		// Check exclude globs.
		for _, pattern := range exclude {
			if pathglob.Match(pattern, rel) {
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
