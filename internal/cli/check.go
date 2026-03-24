package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/engineclient"
	pb "github.com/diktahq/verikt/internal/engineclient/pb"
	"github.com/diktahq/verikt/internal/rules"
	"github.com/spf13/cobra"
)

// ErrCheckFailed is returned when architecture check finds error-severity violations.
var ErrCheckFailed = errors.New("architecture check failed")

type checkFlags struct {
	projectPath string
	proxyRules  bool
	detectors   bool
	decisions   bool
	rule        string
	staged      bool
	diff        string
}

func newCheckCommand(opts *globalOptions) *cobra.Command {
	flags := &checkFlags{}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate project against verikt.yaml rules",
		Long: `Check validates an existing project against its verikt.yaml rules.

Reports dependency violations, structure issues, and function complexity.
Runs both built-in detectors and proxy rules by default.
Exits with code 1 if any error-severity violations are found (useful in CI).`,
		Example: `  verikt check
  verikt check --path ./my-service
  verikt check --proxy-rules
  verikt check --staged
  verikt check --rule cap-sql-parameterized
  verikt check --decisions`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheck(opts, flags)
		},
	}

	cmd.Flags().StringVar(&flags.projectPath, "path", ".", "Project path to check")
	cmd.Flags().BoolVar(&flags.proxyRules, "proxy-rules", false, "Run only proxy rules (skip built-in detectors)")
	cmd.Flags().BoolVar(&flags.detectors, "detectors", false, "Run only built-in detectors (skip proxy rules)")
	cmd.Flags().StringVar(&flags.rule, "rule", "", "Run a single proxy rule by ID")
	cmd.Flags().BoolVar(&flags.staged, "staged", false, "Only check files in git staging area")
	cmd.Flags().BoolVar(&flags.decisions, "decisions", false, "Run only decision gate validation")
	cmd.Flags().StringVar(&flags.diff, "diff", "", "Only report violations in files changed vs. a git ref (e.g., main, HEAD~1)")

	return cmd
}

func runCheck(opts *globalOptions, flags *checkFlags) error {
	projectPath := flags.projectPath

	veriktPath, err := config.FindVeriktYAML(projectPath)
	if err != nil {
		return fmt.Errorf("no verikt.yaml found in %s (or parent directories)", projectPath)
	}

	cfg, err := config.LoadVeriktYAML(veriktPath)
	if err != nil {
		return err
	}

	// Get staged files if --staged is set.
	var stagedFiles []string
	if flags.staged {
		stagedFiles, err = getStagedFiles(projectPath)
		if err != nil {
			return fmt.Errorf("get staged files: %w", err)
		}
		if len(stagedFiles) == 0 {
			fmt.Println("No staged files to check.")
			return nil
		}
	}

	var checkerResult *checker.CheckResult
	var ruleResult *rules.RunResult
	var decisionViolations []checker.DecisionViolation
	hasErrors := false

	// If --decisions is set, only check decisions.
	if flags.decisions {
		decisionViolations = checker.CheckDecisions(cfg.Decisions)
		for _, v := range decisionViolations {
			if v.Severity == "error" {
				hasErrors = true
				break
			}
		}

		if opts.Output == "json" {
			return printCombinedJSON(checkerResult, ruleResult, decisionViolations, hasErrors)
		}
		printCombinedTerminal(checkerResult, ruleResult, decisionViolations, cfg, flags)

		if hasErrors {
			return ErrCheckFailed
		}
		return nil
	}

	// Build engine client once — used for both anti-pattern detection and proxy rules.
	var engineClient *engineclient.Client
	if enginePath, pathErr := engineclient.EnginePath(); pathErr == nil {
		engineClient = engineclient.New(enginePath)
	}

	// Run built-in detectors unless --proxy-rules or --rule is set.
	if !flags.proxyRules && flags.rule == "" {
		var apClient checker.AntiPatternClient
		var depClient checker.DependencyClient
		var metricClient checker.MetricClient
		if engineClient != nil {
			apClient = &engineClientAdapter{engineClient}
			depClient = &engineDepAdapter{client: engineClient, language: cfg.Language}
			metricClient = &engineMetricAdapter{engineClient}
		}
		checkerResult, err = checker.CheckWithEngine(cfg, projectPath, apClient, depClient, metricClient)
		if err != nil {
			return fmt.Errorf("check failed: %w", err)
		}
	}

	// Run proxy rules unless --detectors is set.
	if !flags.detectors {
		rulesDir := filepath.Join(projectPath, ".verikt", "rules")
		ruleResult, err = rules.RunRules(rulesDir, projectPath, stagedFiles, engineClient)
		if err != nil {
			return fmt.Errorf("proxy rules failed: %w", err)
		}

		// Filter to single rule if --rule is set.
		if flags.rule != "" && ruleResult != nil {
			ruleResult = filterRuleResult(ruleResult, flags.rule)
		}
	}

	// Include decision gates if decisions exist in config.
	if len(cfg.Decisions) > 0 {
		decisionViolations = checker.CheckDecisions(cfg.Decisions)
	}

	// Filter to only changed files if --diff is set.
	if flags.diff != "" {
		diffFiles, diffErr := getDiffFiles(projectPath, flags.diff)
		if diffErr != nil {
			return fmt.Errorf("get diff files: %w", diffErr)
		}
		if checkerResult != nil {
			checkerResult = filterCheckerResultByFiles(checkerResult, diffFiles)
		}
		if ruleResult != nil {
			ruleResult = filterRuleResultByFiles(ruleResult, diffFiles)
		}
	}

	// Apply path-scoped severity overrides after all file filtering.
	// Anti-patterns are excluded from overrides (hardcoded safety rules).
	applySeverityOverrides(checkerResult, ruleResult, cfg.SeverityOverrides)

	// Compute hasErrors from final (possibly filtered, overridden) results.
	hasErrors = false
	if checkerResult != nil && !checkerResult.Passed() {
		hasErrors = true
	}
	if ruleResult != nil && ruleResult.ErrorCount() > 0 {
		hasErrors = true
	}
	for _, v := range decisionViolations {
		if v.Severity == "error" {
			hasErrors = true
			break
		}
	}

	if opts.Output == "json" {
		return printCombinedJSON(checkerResult, ruleResult, decisionViolations, hasErrors)
	}
	printCombinedTerminal(checkerResult, ruleResult, decisionViolations, cfg, flags)

	if hasErrors {
		return ErrCheckFailed
	}
	return nil
}

// getStagedFiles returns relative file paths in the git staging area.
func getStagedFiles(projectPath string) ([]string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "diff", "--cached", "--name-only", "--diff-filter=ACM")
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

// getDiffFiles returns relative file paths changed compared to a git ref.
func getDiffFiles(projectPath, ref string) ([]string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "diff", "--name-only", "--", ref)
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s: %w", ref, err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

// fileInSet checks if a file path matches any entry in the set.
// Handles both exact matches and prefix matches for paths that may be absolute or relative.
func fileInSet(file string, set map[string]bool) bool {
	if set[file] {
		return true
	}
	// Violations may have absolute or module-prefixed paths; try the basename match.
	base := filepath.Base(file)
	for f := range set {
		if filepath.Base(f) == base && strings.HasSuffix(file, f) {
			return true
		}
	}
	return false
}

// filterCheckerResultByFiles returns a copy of the result with only violations in the given files.
func filterCheckerResultByFiles(r *checker.CheckResult, files []string) *checker.CheckResult {
	set := make(map[string]bool, len(files))
	for _, f := range files {
		set[f] = true
	}

	filtered := &checker.CheckResult{
		ComponentsCovered: r.ComponentsCovered,
		ComponentsTotal:   r.ComponentsTotal,
		RulesChecked:      r.RulesChecked,
		RulesPassing:      r.RulesPassing,
	}
	for _, v := range r.DependencyViolations {
		if fileInSet(v.File, set) {
			filtered.DependencyViolations = append(filtered.DependencyViolations, v)
		}
	}
	for _, v := range r.StructureViolations {
		if fileInSet(v.File, set) {
			filtered.StructureViolations = append(filtered.StructureViolations, v)
		}
	}
	for _, v := range r.FunctionViolations {
		if fileInSet(v.File, set) {
			filtered.FunctionViolations = append(filtered.FunctionViolations, v)
		}
	}
	for _, v := range r.NamingViolations {
		if fileInSet(v.File, set) {
			filtered.NamingViolations = append(filtered.NamingViolations, v)
		}
	}
	for _, v := range r.AntiPatternViolations {
		if fileInSet(v.File, set) {
			filtered.AntiPatternViolations = append(filtered.AntiPatternViolations, v)
		}
	}
	filtered.RecalculateMetrics()
	return filtered
}

// filterRuleResultByFiles returns a copy of the result with only violations in the given files.
func filterRuleResultByFiles(r *rules.RunResult, files []string) *rules.RunResult {
	set := make(map[string]bool, len(files))
	for _, f := range files {
		set[f] = true
	}

	filtered := &rules.RunResult{Duration: r.Duration, Statuses: r.Statuses}
	for _, v := range r.Violations {
		if fileInSet(v.File, set) {
			filtered.Violations = append(filtered.Violations, v)
		}
	}
	return filtered
}

// applySeverityOverrides applies path-scoped severity overrides to checker and proxy rule violations.
// Anti-pattern violations are intentionally excluded — they are hardcoded safety rules.
func applySeverityOverrides(checkerResult *checker.CheckResult, ruleResult *rules.RunResult, overrides config.SeverityOverrides) {
	if len(overrides) == 0 {
		return
	}

	if checkerResult != nil {
		checkerResult.DependencyViolations = filterViolationsBySeverity(
			checkerResult.DependencyViolations, overrides, func(v checker.Violation) string { return v.Rule })
		checkerResult.StructureViolations = filterViolationsBySeverity(
			checkerResult.StructureViolations, overrides, func(v checker.Violation) string { return v.Rule })
		checkerResult.FunctionViolations = filterViolationsBySeverity(
			checkerResult.FunctionViolations, overrides, func(v checker.Violation) string { return v.Rule })
		checkerResult.NamingViolations = filterViolationsBySeverity(
			checkerResult.NamingViolations, overrides, func(v checker.Violation) string { return v.Rule })
		// AntiPatternViolations are NOT filtered — hardcoded safety rules.
		checkerResult.RecalculateMetrics()
	}

	if ruleResult != nil {
		var filtered []rules.RuleViolation
		for _, v := range ruleResult.Violations {
			resolved := config.ResolveSeverity(overrides, v.RuleID, v.File, v.Severity)
			mapped := config.MapSeverity(resolved)
			if mapped == "ignore" {
				continue
			}
			v.Severity = mapped
			filtered = append(filtered, v)
		}
		ruleResult.Violations = filtered
	}
}

// filterViolationsBySeverity removes violations whose resolved severity is "ignore"
// and updates the severity of remaining violations based on configured overrides.
func filterViolationsBySeverity(violations []checker.Violation, overrides config.SeverityOverrides, keyFn func(checker.Violation) string) []checker.Violation {
	var filtered []checker.Violation
	for _, v := range violations {
		resolved := config.ResolveSeverity(overrides, keyFn(v), v.File, v.Severity)
		mapped := config.MapSeverity(resolved)
		if mapped == "ignore" {
			continue
		}
		v.Severity = mapped
		filtered = append(filtered, v)
	}
	return filtered
}

// filterRuleResult keeps only violations and statuses matching a specific rule ID.
func filterRuleResult(r *rules.RunResult, ruleID string) *rules.RunResult {
	filtered := &rules.RunResult{Duration: r.Duration}
	for _, v := range r.Violations {
		if v.RuleID == ruleID {
			filtered.Violations = append(filtered.Violations, v)
		}
	}
	for _, s := range r.Statuses {
		if s.Rule.ID == ruleID {
			filtered.Statuses = append(filtered.Statuses, s)
		}
	}
	return filtered
}

func printCombinedTerminal(checkerResult *checker.CheckResult, ruleResult *rules.RunResult, decisionViolations []checker.DecisionViolation, cfg *config.VeriktConfig, flags *checkFlags) {
	projectName := cfg.Architecture
	if projectName == "" {
		projectName = "project"
	}

	fmt.Printf("\nverikt check — %s\n", projectName)
	fmt.Println(strings.Repeat("═", 55))

	// Built-in detector results.
	if checkerResult != nil {
		coverage := float64(0)
		if checkerResult.ComponentsTotal > 0 {
			coverage = float64(checkerResult.ComponentsCovered) / float64(checkerResult.ComponentsTotal) * 100
		}
		fmt.Printf("\nComponents:  %d defined, %d covered (%.0f%% coverage)\n",
			checkerResult.ComponentsTotal, checkerResult.ComponentsCovered, coverage)

		archViolations, depViolations := splitByCategory(checkerResult.DependencyViolations, "architecture")
		printViolationSection("ARCHITECTURE VIOLATIONS", archViolations)
		printViolationSection("DEPENDENCY VIOLATIONS", depViolations)
		printViolationSection("STRUCTURE VIOLATIONS", checkerResult.StructureViolations)
		printViolationSection("FUNCTION VIOLATIONS", checkerResult.FunctionViolations)
		printViolationSection("NAMING VIOLATIONS", checkerResult.NamingViolations)
		printAntiPatternSection("ANTI-PATTERN VIOLATIONS", checkerResult.AntiPatternViolations)
	}

	// Proxy rule results.
	if ruleResult != nil {
		printProxyRuleSection(ruleResult)
	}

	// Decision gate results.
	if len(cfg.Decisions) > 0 {
		printDecisionGateSection(cfg.Decisions, decisionViolations)
	}

	fmt.Println()
	fmt.Println(strings.Repeat("═", 55))

	if flags.staged {
		fmt.Println("\nTip: Add to .git/hooks/pre-commit:")
		fmt.Println("  #!/bin/sh")
		fmt.Println("  verikt check --staged")
	}
}

func printProxyRuleSection(result *rules.RunResult) {
	fmt.Printf("\nPROXY RULES (%d valid, %d invalid, %d stale)\n",
		result.ValidRuleCount(), result.InvalidRuleCount(), result.StaleRuleCount())

	if len(result.Violations) == 0 {
		fmt.Println("  ✓ All proxy rules pass")
		return
	}

	errors := result.ErrorCount()
	warnings := result.WarningCount()
	fmt.Printf("  %d errors, %d warnings\n", errors, warnings)

	for _, v := range result.Violations {
		sev := "⚠"
		if v.Severity == "error" {
			sev = "✗"
		}
		if v.Line > 0 {
			fmt.Printf("  %s [%s] %s:%d %s\n", sev, v.RuleID, v.File, v.Line, v.Description)
		} else {
			fmt.Printf("  %s [%s] %s — %s\n", sev, v.RuleID, v.File, v.Description)
		}
		if v.Match != "" {
			fmt.Printf("    > %s\n", v.Match)
		}
	}

	// Report invalid/stale rules.
	for _, s := range result.Statuses {
		switch s.Status {
		case "invalid", "stale":
			fmt.Printf("  ⚠ [%s] %s: %s\n", s.Filename, s.Status, s.Error)
		}
	}
}

// splitByCategory splits violations into those matching category and the rest.
func splitByCategory(violations []checker.Violation, category string) (matched, rest []checker.Violation) {
	for _, v := range violations {
		if v.Category == category {
			matched = append(matched, v)
		} else {
			rest = append(rest, v)
		}
	}
	return matched, rest
}

func printViolationSection(title string, violations []checker.Violation) {
	fmt.Printf("\n%s (%d)\n", title, len(violations))
	if len(violations) == 0 {
		fmt.Println("  ✓ All checks pass")
		return
	}
	for _, v := range violations {
		switch {
		case v.File != "" && v.Line > 0:
			fmt.Printf("  ✗ %s:%d %s\n", v.File, v.Line, v.Message)
		case v.File != "":
			fmt.Printf("  ✗ %s — %s\n", v.File, v.Message)
		default:
			fmt.Printf("  ✗ %s\n", v.Message)
		}
	}
}

func printAntiPatternSection(title string, violations []checker.AntiPattern) {
	fmt.Printf("\n%s (%d)\n", title, len(violations))
	if len(violations) == 0 {
		fmt.Println("  ✓ All checks pass")
		return
	}
	for _, v := range violations {
		sev := "⚠"
		if v.Severity == "error" {
			sev = "✗"
		}
		switch {
		case v.File != "" && v.Line > 0:
			fmt.Printf("  %s [%s] %s:%d %s\n", sev, v.Name, v.File, v.Line, v.Message)
		case v.File != "":
			fmt.Printf("  %s [%s] %s — %s\n", sev, v.Name, v.File, v.Message)
		default:
			fmt.Printf("  %s [%s] %s\n", sev, v.Name, v.Message)
		}
	}
}

func printDecisionGateSection(decisions []config.Decision, violations []checker.DecisionViolation) {
	fmt.Println("\nDECISION GATES")

	// Build a set of undecided topics for quick lookup.
	undecided := make(map[string]checker.DecisionViolation, len(violations))
	for _, v := range violations {
		undecided[v.Topic] = v
	}

	tier1Total, tier1Decided := 0, 0
	tier2Total, tier2Decided := 0, 0

	for _, d := range decisions {
		if d.Tier == 1 {
			tier1Total++
		} else {
			tier2Total++
		}

		if v, ok := undecided[d.Topic]; ok {
			if v.Severity == "error" {
				fmt.Printf("  ✗ %s: UNDECIDED (Tier %d)\n", d.Topic, d.Tier)
			} else {
				fmt.Printf("  ⚠ %s: UNDECIDED (Tier %d)\n", d.Topic, d.Tier)
			}
		} else {
			if d.Tier == 1 {
				tier1Decided++
			} else {
				tier2Decided++
			}
			fmt.Printf("  ✓ %s: %s (Tier %d)\n", d.Topic, d.Choice, d.Tier)
		}
	}

	fmt.Println()
	if tier1Total > 0 {
		blocking := tier1Total - tier1Decided
		if blocking > 0 {
			fmt.Printf("  Tier 1: %d/%d decided (%d blocking)\n", tier1Decided, tier1Total, blocking)
		} else {
			fmt.Printf("  Tier 1: %d/%d decided\n", tier1Decided, tier1Total)
		}
	}
	if tier2Total > 0 {
		fmt.Printf("  Tier 2: %d/%d decided\n", tier2Decided, tier2Total)
	}
}

func printCombinedJSON(checkerResult *checker.CheckResult, ruleResult *rules.RunResult, decisionViolations []checker.DecisionViolation, hasErrors bool) error {
	type jsonOutput struct {
		Result             string                      `json:"result"`
		Violations         []checker.Violation         `json:"violations,omitempty"`
		AntiPatterns       []checker.AntiPattern       `json:"anti_patterns,omitempty"`
		ProxyRules         *rules.RunResult            `json:"proxy_rules,omitempty"`
		DecisionViolations []checker.DecisionViolation `json:"decision_violations,omitempty"`
	}

	status := "pass"
	if hasErrors {
		status = "fail"
	}

	out := jsonOutput{
		Result:             status,
		ProxyRules:         ruleResult,
		DecisionViolations: decisionViolations,
	}

	if checkerResult != nil {
		allViolations := make([]checker.Violation, 0, len(checkerResult.DependencyViolations)+len(checkerResult.StructureViolations)+len(checkerResult.FunctionViolations)+len(checkerResult.NamingViolations))
		allViolations = append(allViolations, checkerResult.DependencyViolations...)
		allViolations = append(allViolations, checkerResult.StructureViolations...)
		allViolations = append(allViolations, checkerResult.FunctionViolations...)
		allViolations = append(allViolations, checkerResult.NamingViolations...)
		out.Violations = allViolations
		out.AntiPatterns = checkerResult.AntiPatternViolations
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// engineDepAdapter wraps *engineclient.Client to satisfy checker.DependencyClient.
// It converts verikt.yaml components into ImportGraphSpec rules and maps findings
// back to checker.Violation.
type engineDepAdapter struct {
	client   *engineclient.Client
	language string // "go", "typescript", etc.
}

func (a *engineDepAdapter) CheckDependencies(projectPath string, components []config.Component) ([]checker.Violation, error) {
	rules := componentsToImportRules(components, a.language)
	if len(rules) == 0 {
		return nil, nil
	}

	result, err := a.client.Check(context.Background(), projectPath, rules, nil)
	if err != nil {
		return nil, err
	}

	out := make([]checker.Violation, 0, len(result.Findings))
	for _, f := range result.Findings {
		out = append(out, checker.Violation{
			Category: "dependency",
			File:     f.File,
			Line:     int(f.Line),
			Message:  f.Message,
			Rule:     f.RuleId,
			Severity: "error",
		})
	}
	return out, nil
}

// componentsToImportRules converts verikt.yaml component dependency rules into
// ImportGraphSpec rules understood by the Rust engine. One rule is generated per
// (component, In pattern) pair, with forbidden = In patterns of components not in
// the component's may_depend_on list.
//
// language is set on the rule scope so the Rust engine uses the correct extractor
// ("typescript" → tree-sitter-typescript; anything else → tree-sitter-go).
func componentsToImportRules(components []config.Component, language string) []*pb.Rule {
	var rules []*pb.Rule

	for _, comp := range components {
		allowedNames := make(map[string]bool, len(comp.MayDependOn))
		for _, name := range comp.MayDependOn {
			allowedNames[name] = true
		}

		var forbidden []string
		for _, other := range components {
			if other.Name == comp.Name || allowedNames[other.Name] {
				continue
			}
			forbidden = append(forbidden, other.In...)
		}

		if len(forbidden) == 0 {
			continue // component has no restrictions
		}

		for _, pkgPattern := range comp.In {
			rules = append(rules, &pb.Rule{
				Id:       "arch/" + comp.Name,
				Severity: pb.Severity_ERROR,
				Message:  comp.Name + " dependency violation",
				Engine:   pb.EngineType_IMPORT_GRAPH,
				Scope:    &pb.RuleScope{Language: language},
				Spec: &pb.Rule_ImportGraph{
					ImportGraph: &pb.ImportGraphSpec{
						PackagePattern: pkgPattern,
						Forbidden:      forbidden,
					},
				},
			})
		}
	}

	return rules
}

// engineMetricAdapter wraps *engineclient.Client to satisfy checker.MetricClient.
type engineMetricAdapter struct {
	client *engineclient.Client
}

func (a *engineMetricAdapter) CheckFunctionMetrics(projectPath string, rules config.FunctionRules) ([]checker.Violation, error) {
	if rules.MaxLines == 0 && rules.MaxParams == 0 && rules.MaxReturnValues == 0 {
		return nil, nil
	}

	rule := &pb.Rule{
		Id:       "function-metrics",
		Severity: pb.Severity_WARNING,
		Message:  "function metric violation",
		Engine:   pb.EngineType_METRIC,
		Spec: &pb.Rule_FunctionMetric{
			FunctionMetric: &pb.FunctionMetricSpec{
				MaxLines:   int32(rules.MaxLines),
				MaxParams:  int32(rules.MaxParams),
				MaxReturns: int32(rules.MaxReturnValues),
			},
		},
	}

	result, err := a.client.Check(context.Background(), projectPath, []*pb.Rule{rule}, nil)
	if err != nil {
		return nil, err
	}

	out := make([]checker.Violation, 0, len(result.Findings))
	for _, f := range result.Findings {
		out = append(out, checker.Violation{
			Category: "function",
			File:     f.File,
			Line:     int(f.Line),
			Message:  f.Message,
			Rule:     f.Match, // Match carries "function_lines" / "function_params" / "function_returns"
			Severity: "warning",
		})
	}
	return out, nil
}

// engineClientAdapter wraps *engineclient.Client to satisfy checker.AntiPatternClient.
// It lives here (not in checker) to avoid an import cycle between checker and engineclient.
type engineClientAdapter struct {
	client *engineclient.Client
}

func (a *engineClientAdapter) CheckAntiPatterns(projectPath string, detectors []string) ([]checker.AntiPattern, error) {
	results, err := a.client.CheckAntiPatterns(projectPath, detectors)
	if err != nil {
		return nil, err
	}
	out := make([]checker.AntiPattern, 0, len(results))
	for _, r := range results {
		out = append(out, checker.AntiPattern{
			Name:     r.Name,
			Category: r.Category,
			Severity: r.Severity,
			File:     r.File,
			Line:     r.Line,
			Message:  r.Message,
		})
	}
	return out, nil
}
