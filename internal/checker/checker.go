package checker

import (
	"errors"
	"fmt"
	"go/ast"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/verikt/internal/analyzer"
	"github.com/diktahq/verikt/internal/analyzer/graph"
	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/provider"
	"golang.org/x/tools/go/packages"
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

// Check validates a project at the given path against its verikt.yaml config.
// It calls CheckWithEngine(cfg, projectPath, nil, nil, nil).
func Check(cfg *config.VeriktConfig, projectPath string) (*CheckResult, error) {
	return CheckWithEngine(cfg, projectPath, nil, nil, nil)
}

// CheckWithEngine is like Check but uses the provided engine clients for
// anti-pattern, dependency, and function metric detection when non-nil.
// When all three clients are non-nil, go/packages loading is skipped entirely.
// TypeScript projects always skip go/packages — the Rust engine handles analysis.
func CheckWithEngine(cfg *config.VeriktConfig, projectPath string, apClient AntiPatternClient, depClient DependencyClient, metricClient MetricClient) (*CheckResult, error) {
	result := &CheckResult{
		ComponentsTotal: len(cfg.Components),
	}

	// TypeScript: no Go packages to load. Use engine-only path when available,
	// or fall back to structure + component coverage checks only.
	if cfg.Language == "typescript" {
		return checkTypeScript(cfg, projectPath, result, apClient, depClient, metricClient)
	}

	// Engine fast path: all three clients available — skip go/packages entirely.
	if apClient != nil && depClient != nil && metricClient != nil {
		return checkWithEngineOnly(cfg, projectPath, result, apClient, depClient, metricClient)
	}

	// Go packages path (fallback or partial).
	a := analyzer.New(projectPath)
	if err := a.LoadPackages(""); err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	depGraph := graph.BuildGraph(a.Packages())

	// Dependency violations.
	if depClient != nil {
		if engineViolations, err := depClient.CheckDependencies(projectPath, cfg.Components); err == nil {
			result.DependencyViolations = engineViolations
		} else {
			result.DependencyViolations = goLayerViolations(depGraph, cfg.Components)
		}
	} else {
		result.DependencyViolations = goLayerViolations(depGraph, cfg.Components)
	}

	// Component coverage (requires package graph).
	result.ComponentsCovered = countCoveredComponents(depGraph, cfg.Components)

	// Architecture shape violations (orphan packages + missing components).
	result.DependencyViolations = append(result.DependencyViolations,
		checkArchitectureShape(cfg, a.Packages(), projectPath)...)

	// Structure violations.
	result.StructureViolations = checkStructure(cfg.Rules.Structure, projectPath)

	// Function violations.
	if metricClient != nil {
		if engineViolations, err := metricClient.CheckFunctionMetrics(projectPath, cfg.Rules.Functions); err == nil {
			result.FunctionViolations = engineViolations
		} else {
			result.FunctionViolations = checkFunctions(cfg.Rules.Functions, a.Packages())
		}
	} else {
		result.FunctionViolations = checkFunctions(cfg.Rules.Functions, a.Packages())
	}

	// Anti-pattern violations.
	if apClient != nil {
		if antiPatterns, err := apClient.CheckAntiPatterns(projectPath, nil); err == nil {
			result.AntiPatternViolations = antiPatterns
		} else {
			result.AntiPatternViolations = checkAntiPatterns(a.Packages(), projectPath)
		}
	} else {
		result.AntiPatternViolations = checkAntiPatterns(a.Packages(), projectPath)
	}

	computeMetrics(cfg, result)
	return result, nil
}

// checkTypeScript runs checks for TypeScript projects.
// It skips Go-specific checks (go/packages, anti-patterns, function metrics) and
// uses the Rust engine for import graph dependency checks when a client is available.
func checkTypeScript(cfg *config.VeriktConfig, projectPath string, result *CheckResult, _ AntiPatternClient, depClient DependencyClient, _ MetricClient) (*CheckResult, error) {
	// Structure and coverage checks are language-agnostic.
	result.StructureViolations = checkStructure(cfg.Rules.Structure, projectPath)
	result.ComponentsCovered = countCoveredComponentsFS(projectPath, cfg.Components)
	result.DependencyViolations = detectMissingComponents(cfg, projectPath)

	// Dependency checks via the Rust engine's TypeScript import graph.
	if depClient != nil {
		if violations, err := depClient.CheckDependencies(projectPath, cfg.Components); err == nil {
			result.DependencyViolations = append(result.DependencyViolations, violations...)
		}
	}

	computeMetrics(cfg, result)
	return result, nil
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

	if len(errs) > 0 {
		// Return partial results with the first error.
		return result, errs[0]
	}

	computeMetrics(cfg, result)
	return result, nil
}

// checkArchitectureShape runs two checks:
//  1. Orphan packages — project-local Go packages that match no declared component.
//  2. Missing components — declared components with no Go files in their paths.
//
// Both are reported as "architecture" category violations. Paths matching
// cfg.Check.Exclude globs are skipped.
func checkArchitectureShape(cfg *config.VeriktConfig, pkgs []*packages.Package, projectPath string) []Violation {
	if len(cfg.Components) == 0 {
		return nil
	}
	violations := make([]Violation, 0, len(cfg.Components))
	localPaths := projectLocalPkgPaths(pkgs, projectPath, cfg.Check.Exclude)
	violations = append(violations, detectOrphanPackages(cfg, localPaths)...)
	violations = append(violations, detectMissingComponents(cfg, projectPath)...)
	return violations
}

// projectLocalPkgPaths returns import paths of packages whose source files
// live under projectPath. This filters out stdlib and third-party dependencies
// (which for Go live in the module cache outside the project directory).
// Paths matching any exclude glob are also removed.
func projectLocalPkgPaths(pkgs []*packages.Package, projectPath string, excludes []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, pkg := range pkgs {
		if pkg.PkgPath == "" || seen[pkg.PkgPath] {
			continue
		}
		if isExcluded(pkg.PkgPath, excludes) {
			continue
		}
		for _, f := range pkg.GoFiles {
			if strings.HasPrefix(f, projectPath) {
				seen[pkg.PkgPath] = true
				result = append(result, pkg.PkgPath)
				break
			}
		}
	}
	return result
}

// detectOrphanPackages finds project-local packages that match no declared
// component. These represent unclassified code — likely a flat structure that
// does not implement the declared architecture.
func detectOrphanPackages(cfg *config.VeriktConfig, localPkgPaths []string) []Violation {
	var violations []Violation
	for _, pkgPath := range localPkgPaths {
		matched := false
		for _, comp := range cfg.Components {
			if graph.MatchesComponent(pkgPath, comp) {
				matched = true
				break
			}
		}
		if !matched {
			violations = append(violations, Violation{
				Category: "architecture",
				File:     pkgPath,
				Message:  fmt.Sprintf("package %q matches no declared component — does not conform to %s architecture", pkgPath, cfg.Architecture),
				Rule:     "orphan_package",
				Severity: "error",
			})
		}
	}
	return violations
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
		if err != nil || !d.IsDir() {
			return err
		}
		// Skip symlinked directories — they point outside the project.
		if d.Type()&fs.ModeSymlink != 0 {
			return filepath.SkipDir
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

		// Skip dirs in check.exclude.
		var importPath string
		if rel == "." {
			importPath = modulePath
		} else {
			importPath = modulePath + "/" + filepath.ToSlash(rel)
		}
		if isExcluded(importPath, cfg.Check.Exclude) {
			return filepath.SkipDir
		}

		// Check if this dir contains any .go files.
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
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
			violations = append(violations, Violation{
				Category: "architecture",
				File:     importPath,
				Message:  fmt.Sprintf("package %q matches no declared component — does not conform to %s architecture", importPath, cfg.Architecture),
				Rule:     "orphan_package",
				Severity: "error",
			})
		}
		return nil
	})
	if err != nil {
		return nil
	}
	return violations
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

// isExcluded returns true if the given path matches any of the exclude globs.
func isExcluded(pkgPath string, excludes []string) bool {
	for _, pattern := range excludes {
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(pattern, "/**")
			if strings.Contains(pkgPath, prefix) {
				return true
			}
			continue
		}
		if ok, _ := filepath.Match(pattern, pkgPath); ok {
			return true
		}
	}
	return false
}

func goLayerViolations(depGraph provider.DependencyGraph, components []config.Component) []Violation {
	violations := make([]Violation, 0, len(components))
	for _, v := range graph.LayerViolations(depGraph, components) {
		violations = append(violations, Violation{
			Category: "dependency",
			File:     v.Source,
			Message:  v.Message,
			Rule:     v.Rule,
			Severity: v.Severity,
		})
	}
	return violations
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

func countCoveredComponents(depGraph provider.DependencyGraph, components []config.Component) int {
	covered := 0
	for _, comp := range components {
		for _, node := range depGraph.Nodes {
			if graph.MatchesComponent(node.Path, comp) {
				covered++
				break
			}
		}
	}
	return covered
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

func checkFunctions(rules config.FunctionRules, pkgs []*packages.Package) []Violation {
	if rules.MaxLines == 0 && rules.MaxParams == 0 && rules.MaxReturnValues == 0 {
		return nil
	}

	var violations []Violation
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			fset := pkg.Fset
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn == nil {
					continue
				}

				filePath := fset.Position(fn.Pos()).Filename
				line := fset.Position(fn.Pos()).Line

				// Check line count.
				if rules.MaxLines > 0 && fn.Body != nil {
					startLine := fset.Position(fn.Body.Lbrace).Line
					endLine := fset.Position(fn.Body.Rbrace).Line
					lines := endLine - startLine
					if lines > rules.MaxLines {
						violations = append(violations, Violation{
							Category: "function",
							File:     filePath,
							Line:     line,
							Message:  fmt.Sprintf("%s — %d lines (max: %d)", fn.Name.Name, lines, rules.MaxLines),
							Rule:     "max_lines",
							Severity: "warning",
						})
					}
				}

				// Check param count.
				if rules.MaxParams > 0 && fn.Type.Params != nil {
					params := fn.Type.Params.NumFields()
					if params > rules.MaxParams {
						violations = append(violations, Violation{
							Category: "function",
							File:     filePath,
							Line:     line,
							Message:  fmt.Sprintf("%s — %d params (max: %d)", fn.Name.Name, params, rules.MaxParams),
							Rule:     "max_params",
							Severity: "warning",
						})
					}
				}

				// Check return value count.
				if rules.MaxReturnValues > 0 && fn.Type.Results != nil {
					results := fn.Type.Results.NumFields()
					if results > rules.MaxReturnValues {
						violations = append(violations, Violation{
							Category: "function",
							File:     filePath,
							Line:     line,
							Message:  fmt.Sprintf("%s — %d return values (max: %d)", fn.Name.Name, results, rules.MaxReturnValues),
							Rule:     "max_return_values",
							Severity: "warning",
						})
					}
				}
			}
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
