package checker

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/verikt/internal/analyzer/graph"
	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/pathglob"
)

// Violation represents a single rule violation.
type Violation struct {
	Category string `json:"category"`
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
}

// CheckResult holds the outcome of all checks.
type CheckResult struct {
	DependencyViolations  []Violation   `json:"dependency_violations"`
	StructureViolations   []Violation   `json:"structure_violations"`
	FunctionViolations    []Violation   `json:"function_violations"`
	NamingViolations      []Violation   `json:"naming_violations"`
	AntiPatternViolations []AntiPattern `json:"anti_pattern_violations"`
	ComponentsCovered     int           `json:"components_covered"`
	ComponentsTotal       int           `json:"components_total"`
	RulesChecked          int           `json:"rules_checked"`
	RulesPassing          int           `json:"rules_passing"`

	// WaivedFindings are findings a severity_overrides entry set to "ignore".
	// They are held apart from the violation lists so counts, metrics and the
	// exit code see only what still blocks, and reported separately so a waiver
	// remains visible with the reason its author gave. A finding that vanishes
	// entirely is indistinguishable from one that was never detected.
	WaivedFindings []WaivedFinding `json:"waived_findings,omitempty"`
}

// WaivedFinding is a violation or detector finding that a severity_overrides
// entry set to "ignore", together with the reason that entry gave.
type WaivedFinding struct {
	Category string `json:"category"` // "dependency", "structure", "function", "naming", "anti_pattern"
	Rule     string `json:"rule"`     // violation.Rule or antipattern.Name
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	Reason   string `json:"reason"`
}

// TotalViolations returns the count of all violations.
func (r *CheckResult) TotalViolations() int {
	return len(r.DependencyViolations) + len(r.StructureViolations) +
		len(r.FunctionViolations) + len(r.NamingViolations) +
		len(r.AntiPatternViolations)
}

// Passed returns true if no violations were found.
func (r *CheckResult) Passed() bool {
	return r.TotalViolations() == 0
}

// RecalculateMetrics re-derives RulesPassing from the current slice lengths.
// Must be called after any filtering operation that removes violations.
func (r *CheckResult) RecalculateMetrics() {
	r.RulesPassing = r.RulesChecked - r.TotalViolations()
	if r.RulesPassing < 0 {
		r.RulesPassing = 0
	}
}

// Compliance returns the ratio of passing rules (0.0 to 1.0).
func (r *CheckResult) Compliance() float64 {
	if r.RulesChecked == 0 {
		return 1.0
	}
	return float64(r.RulesPassing) / float64(r.RulesChecked)
}

// AntiPatternClient is the interface used to run anti-pattern checks via the
// Rust engine. Satisfied by *engineclient.Client; nil falls back to Go AST.
type AntiPatternClient interface {
	CheckAntiPatterns(projectPath string, detectors []string) ([]AntiPattern, error)
}

// DependencyClient is the interface used to run component dependency checks via
// the Rust engine. Nil falls back to the Go packages-based implementation.
type DependencyClient interface {
	CheckDependencies(projectPath string, components []config.Component) ([]Violation, error)
}

// MetricClient is the interface used to run function metric checks (max lines,
// params, return values) via the Rust engine. Nil falls back to Go AST.
type MetricClient interface {
	CheckFunctionMetrics(projectPath string, rules config.FunctionRules) ([]Violation, error)
}

// ErrEngineRequired is returned when analysis is requested without an engine.
//
// ADR-006 made the Rust engine the sole implementation of code analysis, and ADR-011
// keeps it that way. There is deliberately no Go fallback: two implementations of the
// same detector disagreed silently, and which one ran depended on whether the embedded
// binary resolved — invisible to the user.
var ErrEngineRequired = errors.New("analysis engine unavailable: verikt check requires the embedded Rust engine")

// CheckWithEngine validates a project against its verikt.yaml config.
//
// All three engine clients are required. Structure, component coverage and
// architecture-shape checks are filesystem-based and need no engine; dependency,
// function-metric and anti-pattern analysis are the engine's.
func CheckWithEngine(cfg *config.VeriktConfig, projectPath string, apClient AntiPatternClient, depClient DependencyClient, metricClient MetricClient) (*CheckResult, error) {
	result := &CheckResult{
		ComponentsTotal: len(cfg.Components),
	}

	// Required clients are checked before the language branch. TypeScript used to
	// return above this point, so a TypeScript project with every client nil got a
	// clean result — and TypeScript dependency analysis is engine-only, so nothing
	// checked its imports at all. That is the failure ErrEngineRequired exists to
	// prevent, reached by taking the other branch.
	//
	// The requirement differs by language: TypeScript uses only the dependency
	// client, and demanding the Go-only ones would fail a check that could run
	// correctly.
	if cfg.Language == "typescript" {
		if depClient == nil {
			return nil, ErrEngineRequired
		}
		return checkTypeScript(cfg, projectPath, result, apClient, depClient, metricClient)
	}

	if apClient == nil || depClient == nil || metricClient == nil {
		return nil, ErrEngineRequired
	}

	return checkWithEngineOnly(cfg, projectPath, result, apClient, depClient, metricClient)
}

// checkTypeScript runs checks for TypeScript projects.
// It skips Go-specific checks (go/packages, anti-patterns, function metrics) and
// uses the Rust engine for import graph dependency checks when a client is available.
func checkTypeScript(cfg *config.VeriktConfig, projectPath string, result *CheckResult, _ AntiPatternClient, depClient DependencyClient, _ MetricClient) (*CheckResult, error) {
	// Structure and coverage checks are language-agnostic.
	result.StructureViolations = checkStructure(cfg.Rules.Structure, projectPath)
	result.ComponentsCovered = countCoveredComponentsFS(projectPath, cfg.Components)
	result.DependencyViolations = detectMissingComponents(cfg, projectPath)
	result.DependencyViolations = append(result.DependencyViolations,
		detectUnreadablePaths(projectPath, cfg.Check.Exclude)...)

	// Dependency checks via the Rust engine's TypeScript import graph.
	//
	// An engine that could not run has not found zero violations, so its error is
	// returned rather than dropped. Discarding it made a crashed, missing or
	// timed-out engine indistinguishable from a clean project — on the one
	// language where the engine is optional, and therefore the one place
	// ErrEngineRequired does not already cover.
	var depErr error
	violations, err := depClient.CheckDependencies(projectPath, cfg.Components)
	if err != nil {
		depErr = fmt.Errorf("dependency check: %w", err)
	} else {
		result.DependencyViolations = append(result.DependencyViolations, violations...)
	}

	applyExcludes(result, cfg.Check.Exclude)
	computeMetrics(cfg, result)
	return result, depErr
}

// checkWithEngineOnly runs all checks via the Rust engine, skipping go/packages.
func checkWithEngineOnly(cfg *config.VeriktConfig, projectPath string, result *CheckResult, apClient AntiPatternClient, depClient DependencyClient, metricClient MetricClient) (*CheckResult, error) {
	var errs []error

	if violations, err := depClient.CheckDependencies(projectPath, cfg.Components); err == nil {
		result.DependencyViolations = violations
	} else {
		errs = append(errs, fmt.Errorf("dependency check: %w", err))
	}

	// Component coverage via filesystem (no go/packages needed).
	result.ComponentsCovered = countCoveredComponentsFS(projectPath, cfg.Components)

	// Architecture shape checks — both filesystem-based, no go/packages needed.
	result.DependencyViolations = append(result.DependencyViolations,
		detectMissingComponents(cfg, projectPath)...)
	result.DependencyViolations = append(result.DependencyViolations,
		detectOrphanPackagesFS(cfg, projectPath)...)
	result.DependencyViolations = append(result.DependencyViolations,
		detectUnreadablePaths(projectPath, cfg.Check.Exclude)...)

	result.StructureViolations = checkStructure(cfg.Rules.Structure, projectPath)

	if violations, err := metricClient.CheckFunctionMetrics(projectPath, cfg.Rules.Functions); err == nil {
		result.FunctionViolations = violations
	} else {
		errs = append(errs, fmt.Errorf("function metric check: %w", err))
	}

	if antiPatterns, err := apClient.CheckAntiPatterns(projectPath, nil); err == nil {
		result.AntiPatternViolations = antiPatterns
	} else {
		errs = append(errs, fmt.Errorf("anti-pattern check: %w", err))
	}

	// Excludes and metrics are applied even when a check failed, so partial
	// results a caller chooses to render still honour the configuration.
	applyExcludes(result, cfg.Check.Exclude)
	computeMetrics(cfg, result)

	if len(errs) > 0 {
		// Return partial results with the first error.
		return result, errs[0]
	}
	return result, nil
}

// detectMissingComponents finds components declared in verikt.yaml that have
// no Go files in their declared paths. The architecture shape is not implemented.
func detectMissingComponents(cfg *config.VeriktConfig, projectPath string) []Violation {
	var violations []Violation
	for _, comp := range cfg.Components {
		if countCoveredComponentsFS(projectPath, []config.Component{comp}) == 0 {
			violations = append(violations, Violation{
				Category: "architecture",
				Message:  fmt.Sprintf("component %q declared in verikt.yaml but no Go files found in %v", comp.Name, comp.In),
				Rule:     "missing_component",
				Severity: "error",
			})
		}
	}
	return violations
}

// detectOrphanPackagesFS is the filesystem-based version of orphan package
// detection — used when go/packages is not available (engine-only path).
// It reads go.mod for the module path, walks directories containing .go files,
// derives import paths, and checks them against declared components.
func detectOrphanPackagesFS(cfg *config.VeriktConfig, projectPath string) []Violation {
	if len(cfg.Components) == 0 {
		return nil
	}

	modulePath := readModulePath(projectPath)
	if modulePath == "" {
		return nil
	}

	var violations []Violation
	seen := map[string]bool{}

	err := filepath.WalkDir(projectPath, func(path string, d fs.DirEntry, err error) error {
		// Symlinks are never project-local code (INV-002). Tested before the
		// directory filter, because WalkDir reports a symlink from Lstat: IsDir()
		// is false even for a link to a directory, so the check that followed the
		// `!d.IsDir()` return was unreachable and linked directories were skipped
		// only because WalkDir does not follow them.
		// An unreadable path is skipped here and reported by
		// detectUnreadablePaths, which walks the whole tree for exactly this and
		// runs for every language.
		//
		// What matters here is that it is not fatal. This returned the error,
		// which aborted the walk, and the caller then discarded every violation
		// found so far — so one unreadable directory anywhere under the project
		// root turned orphan detection off entirely and reported a clean tree.
		if err != nil {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		// Skip hidden dirs, vendor, and testdata.
		base := d.Name()
		if base != "." && (strings.HasPrefix(base, ".") || base == "vendor" || base == "testdata") {
			return filepath.SkipDir
		}

		rel, err := filepath.Rel(projectPath, path)
		if err != nil {
			return err
		}

		// Skip dirs in check.exclude. The patterns are project-relative, so the
		// path is matched before the module prefix is added — matching the full
		// import path meant the module name was searched too.
		relSlash := filepath.ToSlash(rel)
		if relSlash != "." && isExcluded(relSlash, cfg.Check.Exclude) {
			return filepath.SkipDir
		}

		var importPath string
		if rel == "." {
			importPath = modulePath
		} else {
			importPath = modulePath + "/" + relSlash
		}

		// Check if this dir contains any .go files. An unreadable directory is
		// reported for the same reason as above: not reading it is not the same
		// as finding nothing in it.
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil // reported by detectUnreadablePaths
		}
		hasGo := false
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
				hasGo = true
				break
			}
		}
		if !hasGo || seen[importPath] {
			return nil
		}
		seen[importPath] = true

		// Check if import path matches any declared component.
		matched := false
		for _, comp := range cfg.Components {
			if graph.MatchesComponent(importPath, comp) {
				matched = true
				break
			}
		}
		if !matched {
			// File is the project-relative directory, not the import path. Every
			// path-scoped feature — check.exclude, severity_overrides paths, the
			// --diff and --staged filters — matches File against project-relative
			// globs, so an import path here could never be matched by any of them:
			// a waiver for "internal/stray/**" silently did nothing. The import
			// path is still named in the message.
			violations = append(violations, Violation{
				Category: "architecture",
				File:     relSlash,
				Message:  fmt.Sprintf("package %q matches no declared component — does not conform to %s architecture", importPath, cfg.Architecture),
				Rule:     "orphan_package",
				Severity: "error",
			})
		}
		return nil
	})
	if err != nil {
		// Per-entry errors are skipped inside the callback, so reaching here means
		// the walk itself failed. detectUnreadablePaths reports it; the findings
		// gathered so far are still returned rather than discarded.
		return violations
	}
	return violations
}

// detectUnreadablePaths walks the project and reports every path it cannot read.
//
// A path the tool could not read has not been checked, so reporting nothing for
// it is indistinguishable from checking it and finding nothing — the failure this
// whole package is built to avoid. The engine's walkers discard read errors
// silently, so this is the only thing looking.
//
// It runs for every language and needs no module path: the detection used to live
// inside orphan-package detection, which is Go-only and returns early without a
// go.mod, so a TypeScript project or a Go project before `go mod init` got a clean
// report for a tree that was never read.
//
// excludes are honoured, so a deliberately excluded path that happens to be
// unreadable is not reported.
func detectUnreadablePaths(projectPath string, excludes []string) []Violation {
	var violations []Violation
	reported := map[string]bool{}

	report := func(path string, cause error) {
		if reported[path] {
			return
		}
		reported[path] = true
		violations = append(violations, unreadablePathViolation(projectPath, path, cause))
	}

	err := filepath.WalkDir(projectPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// WalkDir reports an unreadable directory twice: once as an entry in
			// its parent's listing, once when descending into it fails.
			report(path, err)
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil // INV-002: symlinks are not project-local code
		}
		if !d.IsDir() {
			if openErr := checkReadable(path); openErr != nil {
				report(path, openErr)
			}
			return nil
		}
		if skipUnreadableScan(projectPath, path, d.Name(), excludes) {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		report(projectPath, err)
	}

	return violations
}

// checkReadable opens a source file to confirm the analysis could read it.
//
// A file that exists and is listed but cannot be read is the case a directory
// walk cannot see: WalkDir succeeds on it, because Lstat succeeds, and only the
// read fails — inside the engine, which discards the error, so the file is
// silently not analysed.
//
// Only files the analysis reads are opened. An unreadable image is not a gap in
// the analysis, and opening every file in the tree to discover that would be
// wasteful.
func checkReadable(path string) error {
	if !isAnalysedSource(path) {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return file.Close()
}

// skipUnreadableScan reports whether a directory is outside the scan.
func skipUnreadableScan(projectPath, path, name string, excludes []string) bool {
	if path == projectPath {
		return false
	}
	if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
		return true
	}
	rel, err := filepath.Rel(projectPath, path)
	if err != nil {
		return false
	}
	relSlash := filepath.ToSlash(rel)
	return relSlash != "." && isExcluded(relSlash, excludes)
}

// isAnalysedSource reports whether a path is a file the analysis reads.
//
// Kept in step with the engine's collectors: Go sources (excluding tests, which
// detectors never see — INV-004) and TypeScript sources.
func isAnalysedSource(path string) bool {
	name := filepath.Base(path)
	switch {
	case strings.HasSuffix(name, "_test.go"):
		return false
	case strings.HasSuffix(name, ".go"):
		return true
	case strings.HasSuffix(name, ".ts"), strings.HasSuffix(name, ".tsx"):
		return !strings.HasSuffix(name, ".d.ts")
	default:
		return false
	}
}

// unreadablePathViolation reports a path the checker could not read.
//
// Its own severity is error: analysis did not happen there, and reporting
// nothing would be indistinguishable from analysis that found nothing. Where a
// path is legitimately unreadable, severity_overrides can waive it with a reason.
func unreadablePathViolation(projectPath, path string, cause error) Violation {
	rel, relErr := filepath.Rel(projectPath, path)
	if relErr != nil {
		rel = path
	}
	return Violation{
		Category: "architecture",
		File:     filepath.ToSlash(rel),
		Message:  fmt.Sprintf("%s could not be read, so it was not analysed: %v", filepath.ToSlash(rel), cause),
		Rule:     "unreadable_path",
		Severity: "error",
	}
}

// readModulePath reads the module path from go.mod in projectPath.
func readModulePath(projectPath string) string {
	data, err := os.ReadFile(filepath.Join(projectPath, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

// isExcluded returns true if the given project-relative path matches any of the
// exclude globs.
//
// The path must be project-relative: matching was previously done by substring
// containment against full import paths, so "gen/**" excluded anything whose path
// merely contained "gen" — internal/agent among them. applyExcludes then extended
// that to every finding category, so an unrelated exclude could silence a SQL
// injection finding and still print "All checks pass".
func isExcluded(relPath string, excludes []string) bool {
	return pathglob.MatchAny(relPath, excludes)
}

// countCoveredComponentsFS checks component coverage using the filesystem —
// no go/packages required. A component is covered if any directory matching
// one of its In patterns exists under projectPath.
func countCoveredComponentsFS(projectPath string, components []config.Component) int {
	covered := 0
	for _, comp := range components {
		for _, pattern := range comp.In {
			// Strip trailing /** for directory existence check.
			dir := strings.TrimSuffix(pattern, "/**")
			dir = strings.TrimSuffix(dir, "/**")
			dirPath := filepath.Join(projectPath, filepath.FromSlash(dir))
			if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
				covered++
				break
			}
		}
	}
	return covered
}

func computeMetrics(cfg *config.VeriktConfig, result *CheckResult) {
	result.RulesChecked = len(cfg.Components) + structureRuleCount(cfg.Rules.Structure) + functionRuleCount(cfg.Rules.Functions)
	result.RulesPassing = result.RulesChecked - result.TotalViolations()
	if result.RulesPassing < 0 {
		result.RulesPassing = 0
	}
}

func checkStructure(rules config.StructureConfig, projectPath string) []Violation {
	var violations []Violation
	for _, dir := range rules.RequiredDirs {
		dirPath := filepath.Join(projectPath, filepath.FromSlash(strings.TrimSuffix(dir, "/")))
		if _, err := os.Stat(dirPath); errors.Is(err, fs.ErrNotExist) {
			violations = append(violations, Violation{
				Category: "structure",
				Message:  fmt.Sprintf("missing required directory: %s", dir),
				Rule:     "required_dir",
				Severity: "error",
			})
		}
	}
	for _, dir := range rules.ForbiddenDirs {
		dirPath := filepath.Join(projectPath, filepath.FromSlash(strings.TrimSuffix(dir, "/")))
		if _, err := os.Stat(dirPath); err == nil {
			violations = append(violations, Violation{
				Category: "structure",
				Message:  fmt.Sprintf("forbidden directory exists: %s", dir),
				Rule:     "forbidden_dir",
				Severity: "error",
			})
		}
	}
	return violations
}

func structureRuleCount(rules config.StructureConfig) int {
	return len(rules.RequiredDirs) + len(rules.ForbiddenDirs)
}

func functionRuleCount(rules config.FunctionRules) int {
	count := 0
	if rules.MaxLines > 0 {
		count++
	}
	if rules.MaxParams > 0 {
		count++
	}
	if rules.MaxReturnValues > 0 {
		count++
	}
	return count
}
