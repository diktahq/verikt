package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
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

// ErrCheckFailed is returned when architecture check finds a finding at or above
// the fail threshold (error by default, warning with --strict or
// check.fail_on: warning), or when a proxy rule could not run (stale or invalid).
var ErrCheckFailed = errors.New("architecture check failed")

// checkJSONSchemaVersion is the version of the `verikt check -o json` document.
// Bump it on any breaking change to the output shape or key names. Adding keys
// (fail_on and the *_count fields) is not one.
const checkJSONSchemaVersion = 2

// enginePathFn resolves the embedded engine binary. It is a variable so tests
// can simulate an engine that cannot be extracted.
var enginePathFn = engineclient.EnginePath

// checkStderr receives warnings. They never go to stdout, which must stay a
// single JSON document in -o json mode.
var checkStderr io.Writer = os.Stderr

type checkFlags struct {
	projectPath   string
	proxyRules    bool
	detectors     bool
	decisions     bool
	rule          string
	staged        bool
	diff          string
	allowDegraded bool
	strict        bool
}

// checkLongHelp is the long help for `verikt check`.
const checkLongHelp = `Check validates an existing project against its verikt.yaml rules.

Reports dependency violations, structure issues, and function complexity.
Runs both built-in detectors and proxy rules by default.
Exits with code 1 if any error-severity finding is reported, or if a proxy rule
could not run (stale or invalid). Warnings do not affect the exit code unless
--strict is set or verikt.yaml sets check.fail_on: warning. Info findings and
findings waived with severity_overrides never fail. A finding with no severity
counts as an error. The last line of terminal output says why the run passed or
failed; -o json carries the same verdict in result, fail_on and the
error_count, warning_count and info_count fields.

Test files (_test.go) are not analysed, by any check — including dependency
rules. A test legitimately reaches across layers to assemble a fixture, so its
imports are not part of the architecture. Generated files carrying the
"// Code generated ... DO NOT EDIT." marker are skipped for the same reason:
they are not the author's to fix.

The built-in detectors run on the embedded analysis engine. If it cannot be
extracted (an unwritable cache directory, a platform with no engine build), the
check fails rather than skip them. --allow-degraded skips the detectors instead,
with a warning on stderr, and checks only proxy rules and decision gates.`

func newCheckCommand(opts *globalOptions) *cobra.Command {
	flags := &checkFlags{}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate project against verikt.yaml rules",
		Long:  checkLongHelp,
		Example: `  verikt check
  verikt check --path ./my-service
  verikt check --proxy-rules
  verikt check --staged
  verikt check --rule cap-sql-parameterized
  verikt check --decisions
  verikt check --strict`,
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
	cmd.Flags().BoolVar(&flags.allowDegraded, "allow-degraded", false,
		"If the analysis engine is unavailable, skip the built-in detectors (warning on stderr) instead of failing")
	cmd.Flags().BoolVar(&flags.strict, "strict", false,
		"Fail on warning-severity findings too (takes precedence over check.fail_on)")

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

	if err := validateCheckConfig(cfg, veriktPath); err != nil {
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
			failOn, _ := resolveFailOn(cfg, flags.strict)
			return reportNothingStaged(opts.Output, failOn)
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
	engineClient, runDetectors, err := resolveCheckEngine(flags)
	if err != nil {
		return err
	}

	if runDetectors {
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
	waived := applySeverityOverrides(checkerResult, ruleResult, cfg.SeverityOverrides)

	return reportCheck(checkOutcome{
		Checker:   checkerResult,
		Rules:     ruleResult,
		Decisions: decisionViolations,
		Waived:    waived,
	}, cfg, flags, opts.Output)
}

// reportNothingStaged reports a --staged run with nothing staged: nothing was
// checked, so it passes. In -o json mode it is still one JSON document — plain
// text on stdout broke every consumer that parses it.
func reportNothingStaged(output, failOn string) error {
	if output == "json" {
		return printCombinedJSON(checkOutcome{}, checkVerdict{FailOn: failOn})
	}
	fmt.Println("No staged files to check.")
	return nil
}

// validateCheckConfig rejects configuration that would make the run check
// something other than what the user asked for.
func validateCheckConfig(cfg *config.VeriktConfig, veriktPath string) error {
	if err := validateConfigCapabilities(cfg); err != nil {
		return err
	}
	// An invalid threshold is a configuration error. Running anyway would report
	// a verdict at a threshold the user did not ask for.
	if err := config.ValidateFailOn(cfg.Check.FailOn); err != nil {
		return err
	}
	// Same for extra commands: a pipeline or quoted string could never match,
	// so silent_ci_swallow would quietly check less than the user asked for.
	errs := config.ValidateSilentCISwallow(cfg.Check.SilentCISwallow)
	// An override check cannot read would change the verdict without a trace:
	// `severity: info` passed an error under --strict and never reached WAIVED.
	errs = append(errs, config.ValidateCheckOverrides(cfg.SeverityOverrides)...)
	// An allowlist entry outside the project can never match, so cmd_allowlist
	// would check something other than what was written.
	errs = append(errs, config.ValidateAllowedEntrypoints(cfg.Check)...)
	if len(errs) > 0 {
		return fmt.Errorf("%s: %w", veriktPath, errors.Join(errs...))
	}
	return nil
}

// resolveCheckEngine extracts the engine and decides whether the built-in
// detectors run. It returns a nil client when the engine is unavailable and the
// run can proceed without it.
//
// The resolution error used to be discarded. The detectors then failed with a
// generic "requires the embedded Rust engine", and the cause — a cache directory
// the process cannot write, a platform with no engine build — was nowhere in the
// output.
//
// Proxy grep rules keep a Go implementation for platforms with no engine build
// (ADR-006), and the report names which implementation ran, so a run that needs
// only proxy rules proceeds without the engine and without a warning. The
// detectors have no second implementation: without the engine they fail closed,
// unless --allow-degraded asks to skip them, which is announced on stderr so
// JSON on stdout stays parseable.
func resolveCheckEngine(flags *checkFlags) (*engineclient.Client, bool, error) {
	runDetectors := !flags.proxyRules && flags.rule == ""

	enginePath, pathErr := enginePathFn()
	if pathErr == nil {
		return engineclient.New(enginePath), runDetectors, nil
	}
	if !runDetectors {
		return nil, false, nil
	}

	// --detectors with the detectors skipped would run no check at all and
	// report a pass, so the escape hatch does not apply to it.
	if !flags.allowDegraded || flags.detectors {
		return nil, false, fmt.Errorf("%w: %w\n"+
			"The built-in detectors (dependency rules, function metrics, anti-patterns) cannot run without it.\n"+
			"Re-run with --allow-degraded to skip them and check only proxy rules and decision gates "+
			"(not available with --detectors, which would leave nothing to check)",
			checker.ErrEngineRequired, pathErr)
	}

	// A failed write to stderr has nowhere more useful to be reported.
	_, _ = fmt.Fprintf(checkStderr,
		"WARNING: analysis engine unavailable (%v).\n"+
			"WARNING: --allow-degraded is set, so the built-in detectors did not run "+
			"(dependency rules, structure, function metrics, anti-patterns). "+
			"Only proxy rules and decision gates were checked.\n", pathErr)
	return nil, false, nil
}

// checkOutcome bundles the three result sets a single check produces. They are
// always carried together, and passing them separately pushed every function
// that handles them past the project's own parameter limit.
//
// Waived is every finding severity_overrides waived, proxy rules included. It
// lives here rather than only on Checker because Checker is nil when only proxy
// rules run (--proxy-rules, --rule, --allow-degraded without an engine), and a
// proxy waiver recorded on a nil result was a waiver dropped.
type checkOutcome struct {
	Checker   *checker.CheckResult
	Rules     *rules.RunResult
	Decisions []checker.DecisionViolation
	Waived    []checker.WaivedFinding
}

// waived returns the findings to report as waived. A caller that built the
// outcome without Waived still gets what the checker recorded.
func (o checkOutcome) waived() []checker.WaivedFinding {
	if o.Waived != nil {
		return o.Waived
	}
	if o.Checker != nil {
		return o.Checker.WaivedFindings
	}
	return nil
}

// checkVerdict is the pass/fail decision for one run, with the counts that
// explain it.
type checkVerdict struct {
	FailOn       string // config.FailOnError or config.FailOnWarning
	FailOnSource string // what set a warning threshold: "--strict" or "check.fail_on"
	Errors       int
	Warnings     int
	Infos        int
	UnrunRules   int // proxy rules that were stale or invalid
	Failed       bool
}

// add counts one finding and applies config.SeverityFails to it.
func (v *checkVerdict) add(severity string) {
	switch config.NormalizeSeverity(severity) {
	case "error":
		v.Errors++
	case "warning":
		v.Warnings++
	case "info":
		v.Infos++
	}
	if config.SeverityFails(severity, v.FailOn) {
		v.Failed = true
	}
}

// evaluateCheck decides whether the run fails at threshold failOn. Every
// finding source — built-in detectors, anti-patterns, proxy rules, decision
// gates — goes through config.SeverityFails. They used to answer "does this
// fail?" three ways: checker findings by their own loop, proxy rules by
// RunResult.ErrorCount, decision gates by comparing to "error". A proxy rule or
// decision with no severity therefore passed while a checker finding with none
// failed.
//
// A proxy rule that could not run fails at every threshold: it checked nothing,
// so it cannot pass.
func evaluateCheck(outcome checkOutcome, failOn string) checkVerdict {
	v := checkVerdict{FailOn: failOn}
	if outcome.Checker != nil {
		for _, sev := range outcome.Checker.Severities() {
			v.add(sev)
		}
	}
	if outcome.Rules != nil {
		for _, rv := range outcome.Rules.Violations {
			v.add(rv.Severity)
		}
		v.UnrunRules = outcome.Rules.StaleRuleCount() + outcome.Rules.InvalidRuleCount()
		if v.UnrunRules > 0 {
			v.Failed = true
		}
	}
	for _, d := range outcome.Decisions {
		v.add(d.Severity)
	}
	return v
}

// resolveFailOn returns the fail threshold for this run and what set it.
// --strict takes precedence over check.fail_on.
func resolveFailOn(cfg *config.VeriktConfig, strict bool) (failOn, source string) {
	if strict {
		return config.FailOnWarning, "--strict"
	}
	if cfg != nil && cfg.Check.FailOn == config.FailOnWarning {
		return config.FailOnWarning, "check.fail_on"
	}
	return config.FailOnError, ""
}

// summaryLine is the last line of terminal output: the counts, the verdict, and
// why. A run that exits 1 with no reason on screen sends the reader looking for
// a failure in the wrong place.
func (v checkVerdict) summaryLine() string {
	counts := pluralize(v.Errors, "error") + ", " + pluralize(v.Warnings, "warning")
	if v.Infos > 0 {
		counts += fmt.Sprintf(", %d info", v.Infos)
	}

	if !v.Failed {
		if v.Warnings > 0 && v.FailOn != config.FailOnWarning {
			return counts + " — passed (use --strict to fail on warnings)"
		}
		return counts + " — passed"
	}

	var reasons []string
	if v.UnrunRules > 0 {
		reasons = append(reasons, pluralize(v.UnrunRules, "proxy rule")+" did not run")
	}
	if v.FailOn == config.FailOnWarning && v.Warnings > 0 {
		reasons = append(reasons, "warnings fail: "+v.FailOnSource)
	}
	if len(reasons) == 0 {
		return counts + " — failed"
	}
	return counts + " — failed (" + strings.Join(reasons, "; ") + ")"
}

// pluralize formats a count with its noun, adding "s" unless n is 1.
func pluralize(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// reportCheck renders the outcome in the requested format and returns
// ErrCheckFailed if the verdict is a failure.
//
// The verdict is independent of the format. The JSON branch used to return as
// soon as it had printed, so a document saying "result": "fail" still exited 0 —
// and every CI example writes JSON to a file, so a pipeline gating on the exit
// code passed on precisely the runs that found something.
func reportCheck(outcome checkOutcome, cfg *config.VeriktConfig, flags *checkFlags, output string) error {
	failOn, source := resolveFailOn(cfg, flags.strict)
	verdict := evaluateCheck(outcome, failOn)
	verdict.FailOnSource = source

	if output == "json" {
		if err := printCombinedJSON(outcome, verdict); err != nil {
			return err
		}
	} else {
		printCombinedTerminal(outcome, cfg, flags, verdict)
	}

	if verdict.Failed {
		return ErrCheckFailed
	}
	return nil
}

// getStagedFiles returns the staged files, relative to projectPath. Renames
// (R) and typechanges (T) are included: a staged move, or a symlink replaced by
// a regular file, is a file the commit is about to change.
func getStagedFiles(projectPath string) ([]string, error) {
	return gitPaths(projectPath, "diff", "--cached", "--relative", "--name-only", "--diff-filter=ACMRT")
}

// gitPaths runs a git command that lists paths and returns them as written on
// disk, relative to projectPath.
//
// -z makes git print each path verbatim and NUL-terminated. Without it, git
// C-quotes a path with non-ASCII bytes or special characters
// ("domain/caf\303\251.go"), which matches no finding path, so every finding in
// such a file was filtered away. --relative (passed by the callers of diff)
// matters for a project in a subdirectory of the repository: git otherwise
// prints repository-root paths (services/api/domain/d.go) while findings are
// project-relative (domain/d.go), and nothing in a monorepo ever matched.
//
// -z goes straight after the subcommand: anything after a "--" is a pathspec.
func gitPaths(projectPath, subcommand string, args ...string) ([]string, error) {
	full := append([]string{"-c", "core.quotepath=false", subcommand, "-z"}, args...)
	// Every argument is a fixed git option or a hash printed by git itself.
	cmd := exec.CommandContext(context.Background(), "git", full...) //nolint:gosec // G204: fixed git subcommand
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s %s: %w", subcommand, strings.Join(args, " "), err)
	}
	var paths []string
	for p := range strings.SplitSeq(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
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
	// A ref starting with '-' would reach git as an option, not a revision.
	if ref == "" {
		return nil, errors.New("git ref must not be empty")
	}
	if strings.HasPrefix(ref, "-") {
		return nil, fmt.Errorf("invalid git ref %q: must not start with '-'", ref)
	}

	base, err := gitMergeBase(projectPath, ref)
	if err != nil {
		return nil, err
	}

	// ACMRT: deleted files cannot carry findings, but a renamed file can — ACM
	// alone filtered away every finding in a file the branch had moved — and so
	// can a symlink replaced by a regular file (T).
	// The trailing "--" pins base as a revision; nothing after it is a pathspec.
	changed, err := gitPaths(projectPath, "diff", "--relative", "--name-only", "--diff-filter=ACMRT", base, "--")
	if err != nil {
		return nil, err
	}

	// The comparison is with the working tree, so a file not yet added is part of
	// it too. git diff never lists untracked files, and the PostToolUse hook runs
	// `--diff HEAD` right after an agent's Write: every finding in a file the
	// agent had just created was filtered away, and the hook stayed silent on it.
	// --exclude-standard keeps ignored build output out.
	untracked, err := gitPaths(projectPath, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	return mergePaths(changed, untracked), nil
}

// mergePaths returns a followed by the entries of b not already in a.
func mergePaths(a, b []string) []string {
	seen := make(map[string]bool, len(a))
	for _, p := range a {
		seen[p] = true
	}
	for _, p := range b {
		if !seen[p] {
			seen[p] = true
			a = append(a, p)
		}
	}
	return a
}

// gitMergeBase resolves the merge base of ref and HEAD.
//
// An unresolvable ref fails here rather than yielding an empty file list, because
// an empty list filters every finding away and reports a pass.
func gitMergeBase(projectPath, ref string) (string, error) {
	// ref is rejected above when empty or option-like, and no shell is involved.
	cmd := exec.CommandContext(context.Background(), "git", "merge-base", ref, "HEAD") //nolint:gosec // G204: fixed git subcommand, validated ref
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

// dirContainsAny reports whether any of files lies inside dir.
//
// Some structure findings name a directory rather than a file —
// live_dot_claude_fixture reports the .claude/ directory once — while a diff
// lists the files in it. Matching by file alone dropped the finding from the
// one diff that introduced it.
func dirContainsAny(dir string, files []string) bool {
	if dir == "" {
		return false
	}
	prefix := strings.TrimSuffix(dir, "/") + "/"
	for _, f := range files {
		if strings.HasPrefix(f, prefix) {
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
		if fileInSet(v.File, set) || dirContainsAny(v.File, files) {
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
	// Waivers stay visible under --diff/--staged: a finding waived in a changed
	// file must still be listed, or the hook reports a clean edit.
	for _, w := range r.WaivedFindings {
		if fileInSet(w.File, set) || dirContainsAny(w.File, files) {
			filtered.WaivedFindings = append(filtered.WaivedFindings, w)
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
//
// It returns every waived finding, so the report has them whether or not the
// detectors ran; with a checker result they are also recorded on it.
func applySeverityOverrides(checkerResult *checker.CheckResult, ruleResult *rules.RunResult, overrides config.SeverityOverrides) []checker.WaivedFinding {
	if len(overrides) == 0 {
		return nil
	}

	// Proxy-rule waivers are collected first so they can be recorded alongside the
	// checker's, on the same result. They used to be dropped outright: the run
	// printed "✓ All proxy rules pass" and waived[] was empty, which is
	// indistinguishable from a rule that never ran — the exact thing the waiver
	// mechanism exists to avoid.
	waived := applyProxyRuleOverrides(ruleResult, overrides)

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
		// The checker's list already holds its own waivers (inline verikt:allow
		// markers) plus the ones just appended; returning only the new ones hid
		// every inline waiver whenever an override matched.
		return checkerResult.WaivedFindings
	}
	return waived
}

// applyProxyRuleOverrides applies severity_overrides to proxy-rule violations
// and returns the ones it waived.
func applyProxyRuleOverrides(ruleResult *rules.RunResult, overrides config.SeverityOverrides) []checker.WaivedFinding {
	if ruleResult == nil {
		return nil
	}
	var waived []checker.WaivedFinding
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
	return waived
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

func printCombinedTerminal(outcome checkOutcome, cfg *config.VeriktConfig, flags *checkFlags, verdict checkVerdict) {
	checkerResult, ruleResult := outcome.Checker, outcome.Rules
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

	// Waivers cover detectors and proxy rules alike, so they are printed whether
	// or not the detectors ran.
	printWaivedSection(outcome.waived())

	// Decision gate results.
	if len(cfg.Decisions) > 0 {
		printDecisionGateSection(cfg.Decisions, outcome.Decisions)
	}

	fmt.Println()
	fmt.Println(strings.Repeat("═", 55))
	fmt.Printf("\n%s\n", verdict.summaryLine())

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

	// Report invalid/stale rules. These fail the build (see evaluateCheck), so
	// they take the failure marker — a run can otherwise exit 1 with no ✗ on
	// screen, which is the same "looks fine, isn't" problem as the inverse case.
	for _, s := range result.Statuses {
		switch s.Status {
		case "invalid", "stale":
			fmt.Printf("  ✗ [%s] %s: %s\n", s.Filename, s.Status, s.Error)
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
// Only error severity fails the check by default, so warnings must not carry the
// failure marker: 51 warning-level findings rendered as ✗ read as 51 failures in
// a run that exits 0. This lived inline in three print functions and was
// corrected in one of them, which is why the other two kept printing the wrong
// glyph. A finding with no severity fails (config.SeverityFails), so it takes ✗.
// Under --strict a warning keeps ⚠; the summary line says that warnings fail.
func severityMarker(severity string) string {
	if config.NormalizeSeverity(severity) == "error" {
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
			fmt.Printf("  %s %s: UNDECIDED (Tier %d)\n", severityMarker(v.Severity), d.Topic, d.Tier)
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
//
// result is the verdict the exit code carries. fail_on is the threshold it was
// decided at, and the *_count fields count findings by severity after
// severity_overrides, across every category including proxy rules and decision
// gates; waived findings are in none of them.
type jsonOutput struct {
	SchemaVersion      int                         `json:"schema_version"`
	Result             string                      `json:"result"`
	FailOn             string                      `json:"fail_on"`
	ErrorCount         int                         `json:"error_count"`
	WarningCount       int                         `json:"warning_count"`
	InfoCount          int                         `json:"info_count"`
	Violations         []checker.Violation         `json:"violations"`
	AntiPatterns       []checker.AntiPattern       `json:"anti_patterns"`
	Waived             []checker.WaivedFinding     `json:"waived"`
	ProxyRules         *rules.RunResult            `json:"proxy_rules,omitempty"`
	DecisionViolations []checker.DecisionViolation `json:"decision_violations,omitempty"`
}

func printCombinedJSON(outcome checkOutcome, verdict checkVerdict) error {
	status := "pass"
	if verdict.Failed {
		status = "fail"
	}

	checkerResult := outcome.Checker
	out := jsonOutput{
		SchemaVersion:      checkJSONSchemaVersion,
		Result:             status,
		FailOn:             verdict.FailOn,
		ErrorCount:         verdict.Errors,
		WarningCount:       verdict.Warnings,
		InfoCount:          verdict.Infos,
		Violations:         []checker.Violation{},
		AntiPatterns:       []checker.AntiPattern{},
		Waived:             []checker.WaivedFinding{},
		ProxyRules:         outcome.Rules,
		DecisionViolations: outcome.Decisions,
	}

	if checkerResult != nil {
		out.Violations = allViolationsOf(checkerResult)
		if checkerResult.AntiPatternViolations != nil {
			out.AntiPatterns = checkerResult.AntiPatternViolations
		}
	}
	if waived := outcome.waived(); waived != nil {
		out.Waived = waived
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

// functionMetricSpec converts verikt.yaml function limits to the engine's int32
// fields, rejecting values the wire format cannot carry instead of letting them
// wrap into a different limit.
func functionMetricSpec(rules config.FunctionRules) (*pb.FunctionMetricSpec, error) {
	maxLines, err := metricLimit("max_lines", rules.MaxLines)
	if err != nil {
		return nil, err
	}
	maxParams, err := metricLimit("max_params", rules.MaxParams)
	if err != nil {
		return nil, err
	}
	maxReturns, err := metricLimit("max_return_values", rules.MaxReturnValues)
	if err != nil {
		return nil, err
	}
	return &pb.FunctionMetricSpec{MaxLines: maxLines, MaxParams: maxParams, MaxReturns: maxReturns}, nil
}

func metricLimit(field string, v int) (int32, error) {
	if v < 0 || v > math.MaxInt32 {
		return 0, fmt.Errorf("rules.functions.%s must be between 0 and %d, got %d", field, math.MaxInt32, v)
	}
	return int32(v), nil
}

func (a *engineMetricAdapter) CheckFunctionMetrics(projectPath string, rules config.FunctionRules) ([]checker.Violation, error) {
	if rules.MaxLines == 0 && rules.MaxParams == 0 && rules.MaxReturnValues == 0 {
		return nil, nil
	}

	spec, err := functionMetricSpec(rules)
	if err != nil {
		return nil, err
	}

	rule := &pb.Rule{
		Id:       "function-metrics",
		Severity: pb.Severity_WARNING,
		Message:  "function metric violation",
		Engine:   pb.EngineType_METRIC,
		Spec:     &pb.Rule_FunctionMetric{FunctionMetric: spec},
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
