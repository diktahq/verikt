package checker

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/diktahq/verikt/internal/config"
)

// RuleCmdAllowlist is the rule, and severity_overrides key, for a Go
// entrypoint that check.allowed_entrypoints does not list.
const RuleCmdAllowlist = "cmd_allowlist"

// detectUnapprovedEntrypoints reports every Go `package main` directory, in the
// project root or anywhere under cmd/, that check.allowed_entrypoints does not
// list. One finding per directory.
//
// It is opt-in: with no allowed_entrypoints key it reports nothing, because
// without a declared binary surface there is nothing to compare against. It is
// Go-only: `package main` is how Go declares a binary, and no other language
// verikt checks has an equivalent this could read.
//
// The severity is warning. An unapproved entrypoint is certain once the
// allowlist exists — the directory is there and the compiler would build it —
// but INV-005 has every detector earn error on real repositories first.
//
// check.exclude is not applied, by the caller or here: projects exclude cmd/**
// from orphan detection (verikt's own defaults once did), which would switch
// this off in exactly the projects it is for. A specific entrypoint is waived
// with severity_overrides.
func detectUnapprovedEntrypoints(cfg *config.VeriktConfig, projectPath string) []Violation {
	if cfg.Language != "" && cfg.Language != "go" {
		return nil
	}
	allowlist, on := cfg.Check.EntrypointAllowlist()
	if !on {
		return nil
	}
	allowed := make(map[string]bool, len(allowlist))
	for _, e := range allowlist {
		allowed[e] = true
	}

	var violations []Violation
	for _, ep := range findGoEntrypoints(projectPath) {
		if allowed[ep.dir] {
			continue
		}
		violations = append(violations, Violation{
			Category: "structure",
			File:     ep.file,
			Message: fmt.Sprintf("unapproved entrypoint %s: its package main is not listed in check.allowed_entrypoints "+
				"— fold it into an approved binary, or add %q to the list", ep.dir, ep.dir),
			Rule:     RuleCmdAllowlist,
			Severity: "warning",
		})
	}
	return violations
}

// goEntrypoint is a directory holding a Go main package.
type goEntrypoint struct {
	dir  string // project-relative, slash-separated; "." for the root
	file string // project-relative path of the file reported for it
}

// findGoEntrypoints returns the main-package directories in the project root
// and under cmd/: the root first, then cmd/ in walk (lexical) order.
//
// Symlinks are never followed (INV-002), and directories the go tool ignores —
// ".x", "_x", testdata, vendor — are skipped. A directory that cannot be read
// is skipped too; detectUnreadablePaths reports it.
func findGoEntrypoints(projectPath string) []goEntrypoint {
	var out []goEntrypoint
	if file, ok := mainPackageFile(projectPath); ok {
		out = append(out, goEntrypoint{dir: ".", file: file})
	}

	cmdRoot := filepath.Join(projectPath, "cmd")
	// Errors are skipped rather than returned: a missing cmd/ is the common
	// case, and an unreadable one is reported by detectUnreadablePaths.
	_ = filepath.WalkDir(cmdRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		// Before the directory filter: WalkDir reports a symlink from Lstat, so
		// IsDir() is false even for a link to a directory (INV-002).
		if d.Type()&fs.ModeSymlink != 0 || !d.IsDir() {
			return nil
		}
		if p != cmdRoot && goToolIgnoresDir(d.Name()) {
			return filepath.SkipDir
		}
		file, ok := mainPackageFile(p)
		if !ok {
			return nil
		}
		rel, relErr := filepath.Rel(projectPath, p)
		if relErr != nil {
			return nil //nolint:nilerr // p is under projectPath; Rel cannot fail
		}
		rel = filepath.ToSlash(rel)
		out = append(out, goEntrypoint{dir: rel, file: path.Join(rel, file)})
		return nil
	})
	return out
}

// goToolIgnoresDir reports whether the go tool skips a directory of this name
// when matching packages.
func goToolIgnoresDir(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
		name == "testdata" || name == "vendor"
}

// mainPackageFile reports whether dir directly holds a non-test Go file in
// package main, and names the file to report: main.go when it qualifies,
// otherwise the first by name.
//
// Test files are never read (INV-004). A file whose //go:build line uses the
// `ignore` tag is a generator script, not an entrypoint. Other constraints —
// GOOS, GOARCH, custom tags — do not hide one: the binary still exists on the
// platforms it targets.
func mainPackageFile(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var found []string
	for _, e := range entries {
		name := e.Name()
		// IsRegular is false for a symlink, so a linked file is skipped (INV-002).
		if !e.Type().IsRegular() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || goToolIgnoresDir(name) {
			continue
		}
		if isBuildableMainFile(filepath.Join(dir, name)) {
			found = append(found, name)
		}
	}
	if len(found) == 0 {
		return "", false
	}
	for _, name := range found {
		if name == "main.go" {
			return name, true
		}
	}
	return found[0], true // os.ReadDir sorts by name
}

// isBuildableMainFile reports whether file declares package main and is not
// excluded from every build by `//go:build ignore`.
//
// A file whose package clause does not parse does not compile, so it is not an
// entrypoint. Failing the whole check over it would turn one broken file into
// no report at all.
func isBuildableMainFile(file string) bool {
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil || f.Name.Name != "main" {
		return false
	}
	return !buildIgnored(f)
}

// buildIgnored reports whether a //go:build line before the package clause
// references the `ignore` tag.
func buildIgnored(f *ast.File) bool {
	for _, group := range f.Comments {
		if group.Pos() >= f.Package {
			break
		}
		for _, c := range group.List {
			if constraintUsesIgnore(c.Text) {
				return true
			}
		}
	}
	return false
}

// constraintUsesIgnore reports whether a comment is a //go:build line that
// mentions the `ignore` tag anywhere in its expression.
func constraintUsesIgnore(line string) bool {
	if !constraint.IsGoBuild(line) {
		return false
	}
	expr, err := constraint.Parse(line)
	if err != nil {
		return false
	}
	ignored := false
	expr.Eval(func(tag string) bool {
		if tag == "ignore" {
			ignored = true
		}
		return false
	})
	return ignored
}
