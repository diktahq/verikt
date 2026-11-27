package checker

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/diktahq/verikt/internal/checker/scriptguard"
	"github.com/diktahq/verikt/internal/config"
)

// scriptGuardSkipDirs are never descended into: version-control metadata,
// third-party trees the author does not control, and test fixtures, which
// hold the patterns on purpose (INV-005).
var scriptGuardSkipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "testdata": true}

// maxScriptGuardFileSize bounds what the line-based guards read; CI files,
// scripts and Dockerfiles are far smaller.
const maxScriptGuardFileSize = 2 << 20

// detectScriptGuards runs silent_ci_swallow, mutable_install_url and
// invalid_allow_directive over the project's CI, hook, script, build and
// container files. It reads each target once, so a bad marker in a file both
// rules read is reported once.
//
// It runs for every language (ADR-011): these files exist whatever the
// source language. check.exclude applies, symlinks are never followed
// (INV-002), and what cannot be read is skipped rather than failing the
// check — detectUnreadablePaths reports it.
func detectScriptGuards(projectPath string, check config.CheckConfig) []Violation {
	opts := scriptguard.Options{ExtraCommands: check.SilentCISwallow.ExtraCommands}
	var out []Violation
	// Per-entry errors are skipped inside the callback; an unreadable path is
	// reported by detectUnreadablePaths, and a failed walk has found nothing.
	_ = filepath.WalkDir(projectPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		// Before the directory filter: WalkDir reports a symlink from Lstat,
		// so IsDir() is false even for a link to a directory (INV-002).
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, relErr := filepath.Rel(projectPath, p)
		if relErr != nil || rel == "." {
			return nil //nolint:nilerr // p is under projectPath; Rel cannot fail
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if scriptGuardSkipDirs[d.Name()] || isExcluded(rel, check.Exclude) {
				return filepath.SkipDir
			}
			return nil
		}
		kind, ok := scriptguard.Classify(rel, check.MutableInstallURL.IncludeDocs)
		if !ok || isExcluded(rel, check.Exclude) {
			return nil
		}
		if content, ok := readScriptFile(p, d); ok {
			out = append(out, scriptFindings(scriptguard.Scan(rel, kind, content, opts))...)
		}
		return nil
	})
	return out
}

// readScriptFile reads a regular text file. Large, binary and unreadable
// files are skipped: they have shown no evidence (INV-005).
func readScriptFile(p string, d fs.DirEntry) ([]byte, bool) {
	info, err := d.Info()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxScriptGuardFileSize {
		return nil, false
	}
	content, err := os.ReadFile(p)
	if err != nil || bytes.IndexByte(content, 0) >= 0 {
		return nil, false
	}
	return content, true
}

// scriptFindings reports line-based findings as structure warnings. Every
// line-based rule ships at warning severity (INV-005); severity_overrides
// raises it per path.
func scriptFindings(findings []scriptguard.Finding) []Violation {
	out := make([]Violation, 0, len(findings))
	for _, f := range findings {
		out = append(out, Violation{
			Category: "structure", File: f.File, Line: f.Line, Message: f.Message, Rule: f.Rule, Severity: "warning",
		})
	}
	return out
}
