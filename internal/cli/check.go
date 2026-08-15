package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/verikt/internal/checker"
	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/engineclient"
	pb "github.com/diktahq/verikt/internal/engineclient/pb"
	"github.com/diktahq/verikt/internal/rules"
	"github.com/spf13/cobra"
)

// ErrCheckFailed is returned when architecture check finds error-severity
// violations, or when a proxy rule could not run (stale or invalid).
var ErrCheckFailed = errors.New("architecture check failed")

// checkJSONSchemaVersion is the version of the `verikt check -o json` document.
// Bump it on any breaking change to the output shape or key names.
const checkJSONSchemaVersion = 2

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
Exits with code 1 if any error-severity violations are found, or if a proxy rule
could not run (stale or invalid). Warnings do not affect the exit code.

Test files (_test.go) are not analysed, by any check — including dependency
rules. A test legitimately reaches across layers to assemble a fixture, so its
imports are not part of the architecture. Generated files carrying the
"// Code generated ... DO NOT EDIT." marker are skipped for the same reason:
they are not the author's to fix.`,
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

	// Each of these narrows the run to one kind of check, so combining them
	// narrows it to nothing: `--detectors --rule x` skipped the detectors (because
	// --rule is set) and skipped the proxy rules (because --detectors is set),
	// then reported a pass. That is the silent pass --rule's unknown-ID error was
	// added to close, reachable by adding one more flag.
	cmd.MarkFlagsMutuallyExclusive("detectors", "proxy-rules", "rule", "decisions")

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

	if err := validateConfigCapabilities(cfg); err != nil {
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

	// If --decisions is set, only check decisions. It reports through the same
	// path as every other mode: this branch carried its own copy of the render
	// and exit logic, including the JSON branch that returned before the exit
	// code was applied.
	if flags.decisions {
		decisionViolations = checker.CheckDecisions(cfg.Decisions)
		return reportCheck(checkOutcome{Decisions: decisionViolations}, cfg, flags, opts.Output)
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
			ruleResult, err = filterRuleResult(ruleResult, flags.rule)
			if err != nil {
				return err
			}
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

	// --staged narrows detector findings too. The staged file list was passed only
	// to the proxy-rule runner, so `--help`'s "only check files in git staging
	// area" held for rules and not for detectors: the pre-commit hook this command
	// suggests failed on code the developer had not staged, which trains people to
	// bypass the hook.
	if flags.staged && checkerResult != nil {
		checkerResult = filterCheckerResultByFiles(checkerResult, stagedFiles)
	}

	// Apply path-scoped severity overrides after all file filtering, so the
	// verdict is computed from the final results.
	applySeverityOverrides(checkerResult, ruleResult, cfg.SeverityOverrides)

	return reportCheck(checkOutcome{
		Checker:   checkerResult,
		Rules:     ruleResult,
		Decisions: decisionViolations,
	}, cfg, flags, opts.Output)
}

// checkOutcome bundles the three result sets a single check produces. They are
// always carried together, and passing them separately pushed every function
// that handles them past the project's own parameter limit.
type checkOutcome struct {
	Checker   *checker.CheckResult
	Rules     *rules.RunResult
	Decisions []checker.DecisionViolation
}

// blocks reports whether anything in the outcome should fail the build: an
// error-severity finding in any category, or a proxy rule that could not run.
func (o checkOutcome) blocks() bool {
	if hasBlockingFindings(o.Checker) || ruleResultBlocks(o.Rules) {
		return true
	}
	for _, v := range o.Decisions {
		if v.Severity == "error" {
			return true
		}
	}
	return false
}

// reportCheck renders the outcome in the requested format and returns
// ErrCheckFailed if anything blocking was found.
//
// The verdict is independent of the format. The JSON branch used to return as
// soon as it had printed, so a document saying "result": "fail" still exited 0 —
// and every CI example writes JSON to a file, so a pipeline gating on the exit
// code passed on precisely the runs that found something.
func reportCheck(outcome checkOutcome, cfg *config.VeriktConfig, flags *checkFlags, output string) error {
	hasErrors := outcome.blocks()

	if output == "json" {
		if err := printCombinedJSON(outcome.Checker, outcome.Rules, outcome.Decisions, hasErrors); err != nil {
			return err
		}
	} else {
		printCombinedTerminal(outcome.Checker, outcome.Rules, outcome.Decisions, cfg, flags)
	}

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
//
// The comparison is against the merge base, so `--diff main` means "what this
// branch changed" rather than "how this branch differs from main right now" — on
// a branch that has fallen behind, the latter also reports every file main moved
// on. Comparing the merge base with the working tree covers committed and
// uncommitted work alike, which is what a developer running this locally means.
//
// The ref used to be passed after `--`, which makes git read it as a *pathspec*.
// It matched no path, returned nothing, exited 0, and every finding was then
// filtered away — so `--diff`, the documented way to adopt verikt on an existing
// codebase without cleaning up the whole repository first, passed
// unconditionally for every ref.
func getDiffFiles(projectPath, ref string) ([]string, error) {
	base, err := gitMergeBase(projectPath, ref)
	if err != nil {
		return nil, err
	}

	// ACM: deleted files cannot carry findings, matching getStagedFiles.
	cmd := exec.CommandContext(context.Background(),
		"git", "diff", "--name-only", "--diff-filter=ACM", base)
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s: %w", base, err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

// gitMergeBase resolves the merge base of ref and HEAD.
//
// An unresolvable ref fails here rather than yielding an empty file list, because
// an empty list filters every finding away and reports a pass.
func gitMergeBase(projectPath, ref string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "merge-base", ref, "HEAD")
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git merge-base %s HEAD: %w (is %q a valid ref?)", ref, err, ref)
	}
	base := strings.TrimSpace(string(out))
	if base == "" {
		return "", fmt.Errorf("no merge base between %q and HEAD", ref)
	}
	return base, nil
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

// applySeverityOverrides applies path-scoped severity overrides to checker and
// proxy rule violations, including anti-patterns.
//
// Anti-patterns used to be exempt, on the reasoning that a safety rule you can
// switch off is not a safety rule. The cost was higher than the benefit: a
// reviewed and accepted finding — a domain package that is deliberately the
// project's vocabulary, a legacy path scheduled for removal — stayed on the
// report forever, and a section that always contains something nobody will act
// on stops being read. The docs had already promised this worked.
//
// Two things keep the safety property. A waiver requires a reason (enforced in
// config validation), so the justification lands in the diff where a reviewer
// sees it; and a waived finding is still reported, in its own section, rather
// than disappearing.
func applySeverityOverrides(checkerResult *checker.CheckResult, ruleResult *rules.RunResult, overrides config.SeverityOverrides) {
	if len(overrides) == 0 {
		return
	}

	// Proxy-rule waivers are collected first so they can be recorded alongside the
	// checker's, on the same result. They used to be dropped outright: the run
	// printed "✓ All proxy rules pass" and waived[] was empty, which is
	// indistinguishable from a rule that never ran — the exact thing the waiver
	// mechanism exists to avoid.
	var waived []checker.WaivedFinding

	if ruleResult != nil {
		filtered := make([]rules.RuleViolation, 0, len(ruleResult.Violations))
		for _, v := range ruleResult.Violations {
			resolved := config.ResolveSeverity(overrides, v.RuleID, v.File, v.Severity)
			mapped := config.MapSeverity(resolved)
			if mapped == "ignore" {
				waived = append(waived, checker.WaivedFinding{
					Category: "proxy_rule",
					Rule:     v.RuleID,
					File:     v.File,
					Line:     v.Line,
					Message:  v.Description,
					Reason:   config.ResolveReason(overrides, v.RuleID, v.File),
				})
				continue
			}
			v.Severity = mapped
			filtered = append(filtered, v)
		}
		ruleResult.Violations = filtered
	}

	if checkerResult != nil {
		checkerResult.DependencyViolations = filterViolationsBySeverity(
			checkerResult.DependencyViolations, overrides, "dependency", &waived)
		checkerResult.StructureViolations = filterViolationsBySeverity(
			checkerResult.StructureViolations, overrides, "structure", &waived)
		checkerResult.FunctionViolations = filterViolationsBySeverity(
			checkerResult.FunctionViolations, overrides, "function", &waived)
		checkerResult.NamingViolations = filterViolationsBySeverity(
			checkerResult.NamingViolations, overrides, "naming", &waived)
		checkerResult.AntiPatternViolations = filterAntiPatternsBySeverity(
			checkerResult.AntiPatternViolations, overrides, &waived)
		checkerResult.WaivedFindings = append(checkerResult.WaivedFindings, waived...)
		checkerResult.RecalculateMetrics()
	}
}

// filterViolationsBySeverity moves violations whose resolved severity is "ignore"
// into waived and updates the severity of the rest from the configured overrides.
func filterViolationsBySeverity(violations []checker.Violation, overrides config.SeverityOverrides, category string, waived *[]checker.WaivedFinding) []checker.Violation {
	filtered := make([]checker.Violation, 0, len(violations))
	for _, v := range violations {
		resolved := config.ResolveSeverity(overrides, v.Rule, v.File, v.Severity)
		mapped := config.MapSeverity(resolved)
		if mapped == "ignore" {
			*waived = append(*waived, checker.WaivedFinding{
				Category: category,
				Rule:     v.Rule,
				File:     v.File,
				Line:     v.Line,
				Message:  v.Message,
				Reason:   config.ResolveReason(overrides, v.Rule, v.File),
			})
			continue
		}
		v.Severity = mapped
		filtered = append(filtered, v)
	}
	return filtered
}

// filterAntiPatternsBySeverity is the anti-pattern counterpart: the override key
// is the detector name.
func filterAntiPatternsBySeverity(findings []checker.AntiPattern, overrides config.SeverityOverrides, waived *[]checker.WaivedFinding) []checker.AntiPattern {
	filtered := make([]checker.AntiPattern, 0, len(findings))
	for _, ap := range findings {
		resolved := config.ResolveSeverity(overrides, ap.Name, ap.File, ap.Severity)
		mapped := config.MapSeverity(resolved)
		if mapped == "ignore" {
			*waived = append(*waived, checker.WaivedFinding{
				Category: "anti_pattern",
				Rule:     ap.Name,
				File:     ap.File,
				Line:     ap.Line,
				Message:  ap.Message,
				Reason:   config.ResolveReason(overrides, ap.Name, ap.File),
			})
			continue
		}
		ap.Severity = mapped
		filtered = append(filtered, ap)
	}
	return filtered
}

// filterRuleResult keeps only violations and statuses matching a specific rule
// ID, and reports an error when no rule has that ID.
//
// `--rule` with a typo filtered everything away and the run then printed "All
// proxy rules pass" and exited 0. A pipeline pinned to a renamed or deleted rule
// would keep passing indefinitely while enforcing nothing — a rule ID that
// matches nothing is the strongest form of "did not run".
func filterRuleResult(r *rules.RunResult, ruleID string) (*rules.RunResult, error) {
	filtered := &rules.RunResult{Duration: r.Duration}
	for _, v := range r.Violations {
		if v.RuleID == ruleID {
			filtered.Violations = append(filtered.Violations, v)
		}
	}

	known := make([]string, 0, len(r.Statuses))
	for _, s := range r.Statuses {
		known = append(known, s.Rule.ID)
		if s.Rule.ID == ruleID {
			filtered.Statuses = append(filtered.Statuses, s)
		}
	}

	if len(filtered.Statuses) == 0 {
		sort.Strings(known)
		if len(known) == 0 {
			return nil, fmt.Errorf("no rule %q: this project defines no proxy rules", ruleID)
		}
		return nil, fmt.Errorf("no rule %q: available rules are %s", ruleID, strings.Join(known, ", "))
	}

	return filtered, nil
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
		printWaivedSection(checkerResult.WaivedFindings)
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

	// Name the implementation when it is not the engine. The Go implementation is
	// a second one that has disagreed with the engine before, so a reader
	// comparing two runs — or a bug report — needs to know which produced these
	// findings. The engine is the default and goes unremarked.
	if result.GrepEngine == rules.GrepEngineGo {
		fmt.Println("  ! grep rules evaluated by the Go implementation — no engine available for this platform")
	}

	// A stale rule matched no files and an invalid rule failed to load: neither
	// ran, so neither passed. Claiming otherwise hides the failure.
	// No rules at all is not a pass. Printing a green tick for checks that do not
	// exist reads, to anyone scanning the output, as something verified — the
	// same confusion as a stale rule reporting as passing.
	if len(result.Statuses) == 0 {
		fmt.Println("  – no proxy rules defined (.verikt/rules/)")
		return
	}

	unrun := result.StaleRuleCount() + result.InvalidRuleCount()
	if len(result.Violations) == 0 && unrun == 0 {
		fmt.Println("  ✓ All proxy rules pass")
		return
	}

	if len(result.Violations) > 0 {
		fmt.Printf("  %d errors, %d warnings\n", result.ErrorCount(), result.WarningCount())
	}
	if unrun > 0 {
		fmt.Printf("  %d rules did not run — they cannot pass\n", unrun)
	}

	printProxyRuleViolations(result.Violations)

	// Report invalid/stale rules. These fail the build (see ruleResultBlocks), so
	// they take the failure marker — a run can otherwise exit 1 with no ✗ on
	// screen, which is the same "looks fine, isn't" problem as the inverse case.
	for _, s := range result.Statuses {
		switch s.Status {
		case "invalid", "stale":
			fmt.Printf("  ✗ [%s] %s: %s\n", s.Filename, s.Status, s.Error)
		}
	}
}

// hasBlockingFindings reports whether the checker found anything that should
// fail the build: an error-severity finding in any category.
//
// It deliberately ignores warnings. CheckResult.Passed() counts every violation,
// and because anti-patterns are exempt from severity_overrides, one
// warning-level anti-pattern (god_package on a deliberately large domain
// package) made exit 0 unreachable with no waiver available. check --help and
// ErrCheckFailed both document error severity as the gate.
func hasBlockingFindings(result *checker.CheckResult) bool {
	if result == nil {
		return false
	}

	for _, group := range [][]checker.Violation{
		result.DependencyViolations,
		result.StructureViolations,
		result.FunctionViolations,
		result.NamingViolations,
	} {
		for _, v := range group {
			if v.Severity == "error" {
				return true
			}
		}
	}
	for _, ap := range result.AntiPatternViolations {
		if ap.Severity == "error" {
			return true
		}
	}

	return false
}

// ruleResultBlocks reports whether proxy rules should fail the build: an
// error-severity violation, or a rule that could not run at all.
func ruleResultBlocks(result *rules.RunResult) bool {
	if result == nil {
		return false
	}
	return result.ErrorCount() > 0 || result.StaleRuleCount() > 0 || result.InvalidRuleCount() > 0
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

// printProxyRuleViolations lists the findings proxy rules produced.
func printProxyRuleViolations(violations []rules.RuleViolation) {
	for _, v := range violations {
		sev := severityMarker(v.Severity)
		if v.Line > 0 {
			fmt.Printf("  %s [%s] %s:%d %s\n", sev, v.RuleID, v.File, v.Line, v.Description)
		} else {
			fmt.Printf("  %s [%s] %s — %s\n", sev, v.RuleID, v.File, v.Description)
		}
		if v.Match != "" {
			fmt.Printf("    > %s\n", v.Match)
		}
	}
}

// severityMarker returns the glyph for a finding's severity.
//
// Only error severity fails the check, so warnings must not carry the failure
// marker: 51 warning-level findings rendered as ✗ read as 51 failures in a run
// that exits 0. This lived inline in three print functions and was corrected in
// one of them, which is why the other two kept printing the wrong glyph.
func severityMarker(severity string) string {
	if severity == "error" {
		return "✗"
	}
	return "⚠"
}

func printViolationSection(title string, violations []checker.Violation) {
	fmt.Printf("\n%s (%d)\n", title, len(violations))
	if len(violations) == 0 {
		fmt.Println("  ✓ All checks pass")
		return
	}
	for _, v := range violations {
		marker := severityMarker(v.Severity)
		switch {
		case v.File != "" && v.Line > 0:
			fmt.Printf("  %s %s:%d %s\n", marker, v.File, v.Line, v.Message)
		case v.File != "":
			fmt.Printf("  %s %s — %s\n", marker, v.File, v.Message)
		default:
			fmt.Printf("  %s %s\n", marker, v.Message)
		}
	}
}

// printWaivedSection lists findings a severity_overrides entry set to "ignore",
// with the reason its author gave.
//
// Waived findings are shown rather than dropped: a decision someone made and
// justified is worth seeing, and a report that hides it looks identical to one
// where the detector never ran. They do not affect the exit code.
func printWaivedSection(waived []checker.WaivedFinding) {
	if len(waived) == 0 {
		return
	}

	fmt.Printf("\nWAIVED (%d)\n", len(waived))
	for _, w := range waived {
		location := w.File
		if w.Line > 0 {
			location = fmt.Sprintf("%s:%d", w.File, w.Line)
		}
		fmt.Printf("  ○ [%s] %s %s\n", w.Rule, location, w.Message)
		fmt.Printf("    reason: %s\n", w.Reason)
	}
}

func printAntiPatternSection(title string, violations []checker.AntiPattern) {
	fmt.Printf("\n%s (%d)\n", title, len(violations))
	if len(violations) == 0 {
		fmt.Println("  ✓ All checks pass")
		return
	}
	for _, v := range violations {
		sev := severityMarker(v.Severity)
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

// allViolationsOf flattens the four violation categories into the single
// violations[] array the JSON document exposes.
func allViolationsOf(result *checker.CheckResult) []checker.Violation {
	all := make([]checker.Violation, 0,
		len(result.DependencyViolations)+len(result.StructureViolations)+
			len(result.FunctionViolations)+len(result.NamingViolations))
	all = append(all, result.DependencyViolations...)
	all = append(all, result.StructureViolations...)
	all = append(all, result.FunctionViolations...)
	all = append(all, result.NamingViolations...)
	return all
}

// jsonOutput is the shape of `verikt check --output json`.
//
// SchemaVersion 2 gave anti_patterns[] the same lowercase json keys as
// violations[]. Version 1 (unversioned) emitted Go field names there (Severity,
// File, Message, Name, Line), so consumers of the old output need updating —
// check schema_version before parsing.
//
// violations[], anti_patterns[] and waived[] are always present, and always
// arrays. The first two carried omitempty, so a passing project emitted neither
// key and the gate published in the docs — jq '[.violations[], .anti_patterns[]]
// | …' — failed with "Cannot iterate over null" on exactly the projects that had
// nothing wrong with them.
type jsonOutput struct {
	SchemaVersion      int                         `json:"schema_version"`
	Result             string                      `json:"result"`
	Violations         []checker.Violation         `json:"violations"`
	AntiPatterns       []checker.AntiPattern       `json:"anti_patterns"`
	Waived             []checker.WaivedFinding     `json:"waived"`
	ProxyRules         *rules.RunResult            `json:"proxy_rules,omitempty"`
	DecisionViolations []checker.DecisionViolation `json:"decision_violations,omitempty"`
}

func printCombinedJSON(checkerResult *checker.CheckResult, ruleResult *rules.RunResult, decisionViolations []checker.DecisionViolation, hasErrors bool) error {
	status := "pass"
	if hasErrors {
		status = "fail"
	}

	out := jsonOutput{
		SchemaVersion:      checkJSONSchemaVersion,
		Result:             status,
		Violations:         []checker.Violation{},
		AntiPatterns:       []checker.AntiPattern{},
		Waived:             []checker.WaivedFinding{},
		ProxyRules:         ruleResult,
		DecisionViolations: decisionViolations,
	}

	if checkerResult != nil {
		out.Violations = allViolationsOf(checkerResult)
		if checkerResult.AntiPatternViolations != nil {
			out.AntiPatterns = checkerResult.AntiPatternViolations
		}
		if checkerResult.WaivedFindings != nil {
			out.Waived = checkerResult.WaivedFindings
		}
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
