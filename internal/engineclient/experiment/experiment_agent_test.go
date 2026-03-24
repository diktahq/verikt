package experiment

// Agent experiments: call Claude as a subprocess (via `claude -p`),
// measure the effect of verikt guide context injection on code quality.
//
// Run with:
//   VERIKT_EXPERIMENT_AGENT=1 go test -run TestAgent -v -timeout 300s ./internal/engineclient/experiment/
//
// Requirements:
//   - `claude` CLI in PATH (Claude Code installed)
//   - `verikt` binary built at ./bin/verikt (run: go build -o ./bin/verikt ./cmd/verikt/)
//   - VERIKT_EXPERIMENT_AGENT=1 env var to opt in (calls are not free)

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// claudeResult holds the full JSON output from `claude -p --output-format json`.
type claudeResult struct {
	Result      string  `json:"result"`
	DurationMs  int     `json:"duration_ms"`
	DurationAPI int     `json:"duration_api_ms"`
	CostUSD     float64 `json:"total_cost_usd"`
	NumTurns    int     `json:"num_turns"`
	StopReason  string  `json:"stop_reason"`
	Usage       struct {
		InputTokens            int `json:"input_tokens"`
		CacheReadInputTokens   int `json:"cache_read_input_tokens"`
		CacheCreateInputTokens int `json:"cache_creation_input_tokens"`
		OutputTokens           int `json:"output_tokens"`
	} `json:"usage"`
}

// agentMetrics is the full record for one experiment run.
type agentMetrics struct {
	Label           string
	Prompt          string
	Response        string
	InputTokens     int
	CacheReadTokens int
	OutputTokens    int
	TotalTokens     int
	DurationMs      int
	DurationAPIMs   int
	CostUSD         float64
	ViolationsDep   int
	ViolationsFn    int
	ViolationsAP    int
	ViolationsArch  int
	ViolationsTotal int
	CompliancePct   float64
	Passed          bool
	GeneratedFiles  map[string]string
	Packages        []string // top-level package directories found
}

// detectPackages returns the unique top-level directories (package prefixes) in the generated files.
func detectPackages(files map[string]string) []string {
	seen := map[string]bool{}
	for path := range files {
		// e.g. "domain/order.go" → "domain", "adapter/handler/h.go" → "adapter"
		parts := strings.SplitN(path, "/", 2)
		if len(parts) > 1 {
			seen[parts[0]] = true
		} else {
			seen["(root)"] = true
		}
	}
	result := make([]string, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// isHexagonalShape returns true if the package list contains the canonical hexagonal dirs.
func isHexagonalShape(pkgs []string) bool {
	required := map[string]bool{"domain": false, "port": false, "service": false, "adapter": false}
	for _, p := range pkgs {
		if _, ok := required[p]; ok {
			required[p] = true
		}
	}
	for _, found := range required {
		if !found {
			return false
		}
	}
	return true
}

// agentGuardOrSkip skips the test unless VERIKT_EXPERIMENT_AGENT=1.
func agentGuardOrSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("VERIKT_EXPERIMENT_AGENT") == "" {
		t.Skip("set VERIKT_EXPERIMENT_AGENT=1 to run agent experiments (default: claude-code via claude -p, no API key needed)")
	}
}

// veriktBin returns the path to the verikt binary.
func veriktBin(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller path")
	}
	// experiment/ → internal/engineclient/ → internal/ → repo root
	root := filepath.Join(filepath.Dir(filename), "..", "..", "..")
	bin := filepath.Join(root, "bin", "verikt")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("verikt binary not found at %s — run: go build -o ./bin/verikt ./cmd/verikt/", bin)
	}
	return bin
}

// claudeEnv returns a clean environment with CLAUDECODE unset.
func claudeEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			env = append(env, e)
		}
	}
	env = append(env, "CLAUDECODE=") // unset nested-session guard
	return env
}

// fileOutputSystemPrompt is the system prompt that instructs the agent to return
// generated files in the parseable === <filepath> === format. Separated from task
// prompts so task prompts can be written as natural engineer language.
const fileOutputSystemPrompt = `You are a Go engineer. When implementing code, return ONLY the file contents using this exact format with no other text:
=== <filepath> ===
[complete file content]

Include all files needed for a working implementation. Module name will be specified in the task.`

// callClaude calls `claude -p` with the given prompt, returns the parsed result.
func callClaude(t *testing.T, prompt string) claudeResult {
	t.Helper()
	_, err := exec.LookPath("claude")
	require.NoError(t, err, "claude CLI not found in PATH")

	cmd := exec.CommandContext(context.Background(), "claude", "-p", prompt, "--output-format", "json", "--system-prompt", fileOutputSystemPrompt)
	cmd.Env = claudeEnv()
	out, err := cmd.Output()
	require.NoError(t, err, "claude -p failed")

	var result claudeResult
	require.NoError(t, json.Unmarshal(out, &result), "parse claude JSON output")
	return result
}

// callClaudeWithTools calls `claude -p` with tool access, running in the given directory.
// The agent can Read, Glob, Grep, Edit, and Write files in the working directory.
func callClaudeWithTools(t *testing.T, prompt, cwd string) claudeResult {
	t.Helper()
	_, err := exec.LookPath("claude")
	require.NoError(t, err, "claude CLI not found in PATH")

	cmd := exec.CommandContext(context.Background(), "claude",
		"-p", prompt,
		"--output-format", "json",
		"--allowedTools", "Read,Glob,Grep,Edit,Write",
		"--dangerously-skip-permissions",
	)
	cmd.Dir = cwd
	cmd.Env = claudeEnv()
	out, err := cmd.Output()
	require.NoError(t, err, "claude -p with tools failed: %s", string(out))

	var result claudeResult
	require.NoError(t, json.Unmarshal(out, &result), "parse claude JSON output")
	return result
}

// runAgentExperimentWithTools copies a fixture to a temp dir, optionally prepends the guide,
// calls Claude with tool access (Read/Glob/Grep/Edit/Write), and runs verikt check.
// Unlike runAgentExperimentWithFixture, the agent reads and modifies files directly.
func runAgentExperimentWithTools(t *testing.T, label, fixtureName, taskPrompt string, withGuide bool) agentMetrics {
	t.Helper()
	bin := veriktBin(t)

	dir := t.TempDir()
	copyDir(t, testdataPath(t, fixtureName), dir)

	// Init git repo and commit baseline so --diff HEAD works after agent edits.
	gitInit(t, dir)

	prompt := taskPrompt
	if withGuide {
		guide := generateGuide(t, bin, dir)
		prompt = guide + "\n\n---\n\n" + taskPrompt
	}

	t.Logf("[%s] calling Claude with tools (cwd=%s)...", label, dir)
	cr := callClaudeWithTools(t, prompt, dir)

	t.Logf("[%s] Claude finished in %dms (api: %dms) — $%.4f | turns=%d",
		label, cr.DurationMs, cr.DurationAPI, cr.CostUSD, cr.NumTurns)
	t.Logf("[%s] tokens: input=%d cache_read=%d output=%d",
		label, cr.Usage.InputTokens, cr.Usage.CacheReadInputTokens, cr.Usage.OutputTokens)

	// Run verikt check with --diff HEAD to only see violations in agent-changed files.
	dep, fn, ap, arch, compliance, passed := runVeriktCheckDiff(t, bin, dir)
	total := dep + fn + ap + arch
	t.Logf("[%s] verikt check --diff HEAD: dep=%d fn=%d ap=%d arch=%d total=%d passed=%v",
		label, dep, fn, ap, arch, total, passed)

	// Also log full check for reference.
	depFull, fnFull, apFull, archFull, _, _ := runVeriktCheck(t, bin, dir)
	totalFull := depFull + fnFull + apFull + archFull
	t.Logf("[%s] verikt check (full): dep=%d fn=%d ap=%d arch=%d total=%d",
		label, depFull, fnFull, apFull, archFull, totalFull)

	return agentMetrics{
		Label:           label,
		Prompt:          taskPrompt,
		Response:        cr.Result,
		InputTokens:     cr.Usage.InputTokens,
		CacheReadTokens: cr.Usage.CacheReadInputTokens,
		OutputTokens:    cr.Usage.OutputTokens,
		TotalTokens:     cr.Usage.InputTokens + cr.Usage.CacheReadInputTokens + cr.Usage.OutputTokens,
		DurationMs:      cr.DurationMs,
		DurationAPIMs:   cr.DurationAPI,
		CostUSD:         cr.CostUSD,
		ViolationsDep:   dep,
		ViolationsFn:    fn,
		ViolationsAP:    ap,
		ViolationsArch:  arch,
		ViolationsTotal: total,
		CompliancePct:   compliance * 100,
		Passed:          passed,
		GeneratedFiles:  nil,
		Packages:        nil,
	}
}

// gitInit initializes a git repo in dir and commits all files as baseline.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"-c", "user.name=test", "-c", "user.email=test@test.com", "commit", "--allow-empty", "-m", "init"},
		{"add", "-A"},
		{"-c", "user.name=test", "-c", "user.email=test@test.com", "commit", "-m", "baseline"},
	} {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// runVeriktCheckDiff runs `verikt check --diff HEAD -o json`.
func runVeriktCheckDiff(t *testing.T, bin, projectDir string) (int, int, int, int, float64, bool) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), bin, "check", "--diff", "HEAD", "-o", "json")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		t.Logf("verikt check --diff error: %v", err)
		return 0, 0, 0, 0, 0, false
	}

	var result checkResult
	if len(out) > 0 {
		if jsonErr := json.Unmarshal(out, &result); jsonErr != nil {
			t.Logf("verikt check --diff parse error: %v\noutput: %s", jsonErr, out)
			return 0, 0, 0, 0, 0, false
		}
	}

	passed := result.Result == "pass"
	dep, fn, arch := 0, 0, 0
	for _, v := range result.Violations {
		switch v.Category {
		case "dependency":
			dep++
		case "function":
			fn++
		case "architecture":
			arch++
		}
	}
	ap := len(result.AntiPatterns)
	compliance := 0.0
	if passed {
		compliance = 1.0
	}
	return dep, fn, ap, arch, compliance, passed
}

// parseGeneratedFiles extracts files from Claude's response.
// Expected format:
//
//	=== path/to/file.go ===
//	[content]
func parseGeneratedFiles(response string) map[string]string {
	files := map[string]string{}
	parts := strings.Split(response, "===")
	for i := 1; i+1 < len(parts); i += 2 {
		path := strings.TrimSpace(parts[i])
		content := strings.TrimSpace(parts[i+1])
		if path != "" && content != "" {
			files[path] = content
		}
	}
	return files
}

// writeProject writes generated files to a temp dir alongside a go.mod and verikt.yaml.
func writeProject(t *testing.T, files map[string]string, verikt string) string {
	t.Helper()
	dir := t.TempDir()

	// Write go.mod.
	goMod := "module example.com/experiment\n\ngo 1.24\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644))

	// Write verikt.yaml.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "verikt.yaml"), []byte(verikt), 0644))

	// Write generated files.
	for path, content := range files {
		full := filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0644))
	}
	return dir
}

// checkResult matches the `verikt check -o json` output schema.
type checkResult struct {
	Result     string `json:"result"` // "pass" | "fail"
	Violations []struct {
		Category string `json:"category"` // "dependency" | "function" | "architecture"
	} `json:"violations"`
	AntiPatterns []any `json:"anti_patterns"`
}

// runVeriktCheck runs `verikt check -o json` and returns (dep, fn, ap, arch, compliance, passed).
func runVeriktCheck(t *testing.T, bin, projectDir string) (int, int, int, int, float64, bool) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), bin, "check", "-o", "json")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		t.Logf("verikt check error: %v", err)
		return 0, 0, 0, 0, 0, false
	}

	var result checkResult
	if len(out) > 0 {
		if jsonErr := json.Unmarshal(out, &result); jsonErr != nil {
			t.Logf("verikt check parse error: %v\noutput: %s", jsonErr, out)
			return 0, 0, 0, 0, 0, false
		}
	}

	passed := result.Result == "pass"
	dep, fn, arch := 0, 0, 0
	for _, v := range result.Violations {
		switch v.Category {
		case "dependency":
			dep++
		case "function":
			fn++
		case "architecture":
			arch++
		}
	}
	ap := len(result.AntiPatterns)
	compliance := 0.0
	if passed {
		compliance = 1.0
	}
	return dep, fn, ap, arch, compliance, passed
}

// generateGuide runs `verikt guide --target cursor` in dir and returns the guide text.
func generateGuide(t *testing.T, bin, dir string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), bin, "guide", "--target", "cursor")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "verikt guide failed: %s", out)

	guidePath := filepath.Join(dir, ".cursorrules")
	guide, err := os.ReadFile(guidePath)
	require.NoError(t, err, "guide file not found after verikt guide")
	return string(guide)
}

// generateGuideFromYAML writes an verikt.yaml to a temp dir, runs `verikt guide`, and returns the text.
func generateGuideFromYAML(t *testing.T, bin, veriktYAML string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "verikt.yaml"), []byte(veriktYAML), 0644))
	return generateGuide(t, bin, dir)
}

// copyDir copies src directory tree to dst.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip symlinks — they may point outside the project (INV-002).
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, data, 0644)
	})
	require.NoError(t, err, "copyDir %s → %s", src, dst)
}

// testdataPath returns the absolute path to a named fixture under testdata/.
func testdataPath(t *testing.T, name string) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(filename), "testdata", name)
}

// runAgentExperimentWithFixture copies a fixture to a temp dir, optionally prepends the guide,
// calls Claude, writes the agent's output files on top of the fixture, and runs verikt check.
func runAgentExperimentWithFixture(t *testing.T, label, fixtureName, taskPrompt string, withGuide bool) agentMetrics {
	t.Helper()
	bin := veriktBin(t)

	// Copy fixture to fresh temp dir so runs are isolated.
	dir := t.TempDir()
	copyDir(t, testdataPath(t, fixtureName), dir)

	prompt := taskPrompt
	if withGuide {
		guide := generateGuide(t, bin, dir)
		prompt = guide + "\n\n---\n\n" + taskPrompt
	}

	t.Logf("[%s] calling Claude...", label)
	cr := callClaude(t, prompt)

	files := parseGeneratedFiles(cr.Result)
	t.Logf("[%s] Claude returned %d files in %dms (api: %dms) — $%.4f",
		label, len(files), cr.DurationMs, cr.DurationAPI, cr.CostUSD)
	t.Logf("[%s] tokens: input=%d cache_read=%d output=%d",
		label, cr.Usage.InputTokens, cr.Usage.CacheReadInputTokens, cr.Usage.OutputTokens)

	// Write agent files on top of the fixture.
	for path, content := range files {
		full := filepath.Join(dir, path)
		if mkErr := os.MkdirAll(filepath.Dir(full), 0755); mkErr != nil {
			t.Logf("mkdir error for %s: %v", path, mkErr)
			continue
		}
		if wErr := os.WriteFile(full, []byte(content), 0644); wErr != nil {
			t.Logf("write error for %s: %v", path, wErr)
		}
	}

	dep, fn, ap, arch, compliance, passed := runVeriktCheck(t, bin, dir)
	total := dep + fn + ap + arch
	t.Logf("[%s] verikt check: dep=%d fn=%d ap=%d arch=%d total=%d passed=%v",
		label, dep, fn, ap, arch, total, passed)

	return agentMetrics{
		Label:           label,
		Prompt:          taskPrompt, // store task only, not prepended guide
		Response:        cr.Result,
		InputTokens:     cr.Usage.InputTokens,
		CacheReadTokens: cr.Usage.CacheReadInputTokens,
		OutputTokens:    cr.Usage.OutputTokens,
		TotalTokens:     cr.Usage.InputTokens + cr.Usage.CacheReadInputTokens + cr.Usage.OutputTokens,
		DurationMs:      cr.DurationMs,
		DurationAPIMs:   cr.DurationAPI,
		CostUSD:         cr.CostUSD,
		ViolationsDep:   dep,
		ViolationsFn:    fn,
		ViolationsAP:    ap,
		ViolationsArch:  arch,
		ViolationsTotal: total,
		CompliancePct:   compliance * 100,
		Passed:          passed,
		GeneratedFiles:  files,
		Packages:        detectPackages(files),
	}
}

// runAgentExperiment runs one full experiment: prompt → Claude → write project → verikt check.
func runAgentExperiment(t *testing.T, label, prompt, veriktYAML string) agentMetrics {
	t.Helper()
	bin := veriktBin(t)

	t.Logf("[%s] calling Claude...", label)
	start := time.Now()
	cr := callClaude(t, prompt)
	_ = time.Since(start)

	files := parseGeneratedFiles(cr.Result)
	t.Logf("[%s] Claude returned %d files in %dms (api: %dms) — $%.4f",
		label, len(files), cr.DurationMs, cr.DurationAPI, cr.CostUSD)
	t.Logf("[%s] tokens: input=%d cache_read=%d output=%d",
		label, cr.Usage.InputTokens, cr.Usage.CacheReadInputTokens, cr.Usage.OutputTokens)

	projectDir := writeProject(t, files, veriktYAML)
	dep, fn, ap, arch, compliance, passed := runVeriktCheck(t, bin, projectDir)

	total := dep + fn + ap + arch
	t.Logf("[%s] verikt check: dep=%d fn=%d ap=%d arch=%d total=%d compliance=%.0f%% passed=%v",
		label, dep, fn, ap, arch, total, compliance*100, passed)

	return agentMetrics{
		Label:           label,
		Prompt:          prompt,
		Response:        cr.Result,
		InputTokens:     cr.Usage.InputTokens,
		CacheReadTokens: cr.Usage.CacheReadInputTokens,
		OutputTokens:    cr.Usage.OutputTokens,
		TotalTokens:     cr.Usage.InputTokens + cr.Usage.CacheReadInputTokens + cr.Usage.OutputTokens,
		DurationMs:      cr.DurationMs,
		DurationAPIMs:   cr.DurationAPI,
		CostUSD:         cr.CostUSD,
		ViolationsDep:   dep,
		ViolationsFn:    fn,
		ViolationsAP:    ap,
		ViolationsArch:  arch,
		ViolationsTotal: total,
		CompliancePct:   compliance * 100,
		Passed:          passed,
		GeneratedFiles:  files,
		Packages:        detectPackages(files),
	}
}

// logContrast prints a side-by-side table of two experiment runs.
func logContrast(t *testing.T, without, with agentMetrics) {
	t.Helper()
	t.Logf("")
	t.Logf("=== Guide → Enforce: Agent Experiment Results ===")
	t.Logf("")
	t.Logf("  %-28s  %-16s  %-16s", "Metric", "Without Guide", "With Guide")
	t.Logf("  %-28s  %-16s  %-16s", strings.Repeat("-", 28), strings.Repeat("-", 16), strings.Repeat("-", 16))
	t.Logf("  %-28s  %-16v  %-16v", "Passed", without.Passed, with.Passed)
	t.Logf("  %-28s  %-16s  %-16s", "Compliance",
		fmt.Sprintf("%.0f%%", without.CompliancePct),
		fmt.Sprintf("%.0f%%", with.CompliancePct))
	t.Logf("  %-28s  %-16d  %-16d", "Dependency violations", without.ViolationsDep, with.ViolationsDep)
	t.Logf("  %-28s  %-16d  %-16d", "Function violations", without.ViolationsFn, with.ViolationsFn)
	t.Logf("  %-28s  %-16d  %-16d", "Anti-pattern violations", without.ViolationsAP, with.ViolationsAP)
	t.Logf("  %-28s  %-16d  %-16d", "Total violations", without.ViolationsTotal, with.ViolationsTotal)
	t.Logf("  %-28s  %-16d  %-16d", "Input tokens", without.InputTokens, with.InputTokens)
	t.Logf("  %-28s  %-16d  %-16d", "Cache read tokens", without.CacheReadTokens, with.CacheReadTokens)
	t.Logf("  %-28s  %-16d  %-16d", "Output tokens", without.OutputTokens, with.OutputTokens)
	t.Logf("  %-28s  %-16s  %-16s", "API latency",
		fmt.Sprintf("%dms", without.DurationAPIMs),
		fmt.Sprintf("%dms", with.DurationAPIMs))
	t.Logf("  %-28s  %-16s  %-16s", "Cost",
		fmt.Sprintf("$%.4f", without.CostUSD),
		fmt.Sprintf("$%.4f", with.CostUSD))
	t.Logf("  %-28s  %-16d  %-16d", "Files generated", len(without.GeneratedFiles), len(with.GeneratedFiles))
	t.Logf("  %-28s  %-16v  %-16v", "Hexagonal shape",
		isHexagonalShape(without.Packages), isHexagonalShape(with.Packages))
	t.Logf("  %-28s  %-16s  %-16s", "Packages",
		strings.Join(without.Packages, ","), strings.Join(with.Packages, ","))
	t.Logf("")

	guideCacheOverhead := with.CacheReadTokens - without.CacheReadTokens
	violationsEliminated := without.ViolationsTotal - with.ViolationsTotal
	t.Logf("  Guide cache overhead: +%d cached tokens", guideCacheOverhead)
	if violationsEliminated > 0 {
		t.Logf("  Violations eliminated: %d", violationsEliminated)
	}
	if !isHexagonalShape(without.Packages) && isHexagonalShape(with.Packages) {
		t.Logf("  Architecture enforced: guide produced hexagonal structure, no-guide did not")
	}
	t.Logf("")
}

const hexagonalVeriktYAML = `language: go
architecture: hexagonal

components:
  - name: domain
    in: [domain/**]
  - name: ports
    in: [port/**]
    may_depend_on: [domain]
  - name: service
    in: [service/**]
    may_depend_on: [domain, ports]
  - name: adapter
    in: [adapter/**]
    may_depend_on: [domain, ports, service]

rules:
  functions:
    max_lines: 50
    max_params: 4
    max_return_values: 2
`

// taskPrompt is deliberately ambiguous — it describes the business problem only,
// NOT the architecture. This is the realistic scenario: an agent gets a task ticket
// with requirements, not an architecture blueprint.
//
// Without guide: Claude chooses its own structure (often violates hexagonal rules).
// With guide: The .cursorrules context steers Claude toward the mandated architecture.
// taskPrompt is the task for EXP-07 / EXP-08: "Same task, different architecture" and
// "Three engineers, one standard". Written as a realistic engineer ticket — no format
// instructions (those are in fileOutputSystemPrompt / system prompt).
const taskPrompt = `Build an order management service in Go.

Orders have a customer name and a total amount in cents. Need POST /orders to create one
and GET /orders/{id} to retrieve it. Validate that customer name is not empty.

Module name: example.com/experiment`

// TestAgent_WithoutGuide runs the task with no architectural context.
// Baseline: what does Claude produce when it has no guide?
func TestAgent_WithoutGuide(t *testing.T) {
	agentGuardOrSkip(t)

	m := runAgentExperiment(t, "without-guide", taskPrompt, hexagonalVeriktYAML)

	t.Logf("=== Without Guide: Prompt ===")
	t.Logf("%s", m.Prompt)
	t.Logf("")
	t.Logf("=== Without Guide: Response ===")
	t.Logf("%s", m.Response)
}

// TestAgent_WithGuide runs the same task with the verikt guide prepended as context.
// The guide is what `verikt guide` generates — 311 lines of architecture instructions.
func TestAgent_WithGuide(t *testing.T) {
	agentGuardOrSkip(t)

	// Read the guide generated from the conforming project's verikt.yaml.
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	guidePath := filepath.Join(filepath.Dir(filename), "testdata", "conforming-hexagonal", ".cursorrules")
	guideBytes, err := os.ReadFile(guidePath)
	require.NoError(t, err, "guide not found — run: verikt guide --target cursor from testdata/conforming-hexagonal/")

	guide := string(guideBytes)
	promptWithGuide := guide + "\n\n---\n\n" + taskPrompt

	m := runAgentExperiment(t, "with-guide", promptWithGuide, hexagonalVeriktYAML)

	t.Logf("=== With Guide: Prompt (guide omitted for brevity, task shown) ===")
	t.Logf("%s", taskPrompt)
	t.Logf("")
	t.Logf("=== With Guide: Response ===")
	t.Logf("%s", m.Response)
}

// TestAgent_Contrast runs both conditions and logs the side-by-side comparison.
// This is the blog post experiment — same task, same enforcement, two outcomes.
func TestAgent_Contrast(t *testing.T) {
	agentGuardOrSkip(t)

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	guidePath := filepath.Join(filepath.Dir(filename), "testdata", "conforming-hexagonal", ".cursorrules")
	guideBytes, err := os.ReadFile(guidePath)
	require.NoError(t, err, "guide not found — run: verikt guide --target cursor from testdata/conforming-hexagonal/")

	guide := string(guideBytes)

	without := runAgentExperiment(t, "without-guide", taskPrompt, hexagonalVeriktYAML)
	with := runAgentExperiment(t, "with-guide", guide+"\n\n---\n\n"+taskPrompt, hexagonalVeriktYAML)

	logContrast(t, without, with)

	// Save results for the Obsidian page.
	saveAgentResults(t, without, with)
}

// TestAgent_Consistency runs each condition 3 times to measure variance.
// The guide should reduce variance — predictability is the claim.
func TestAgent_Consistency(t *testing.T) {
	agentGuardOrSkip(t)

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	guidePath := filepath.Join(filepath.Dir(filename), "testdata", "conforming-hexagonal", ".cursorrules")
	guideBytes, err := os.ReadFile(guidePath)
	require.NoError(t, err)
	guide := string(guideBytes)

	runs := 3
	t.Logf("=== Consistency: %d runs per condition ===", runs)

	type summary struct {
		violations []int
		passed     []bool
	}
	withoutSummary := summary{}
	withSummary := summary{}

	for i := range runs {
		t.Logf("--- Run %d/%d ---", i+1, runs)
		wo := runAgentExperiment(t, fmt.Sprintf("without-guide-run%d", i+1), taskPrompt, hexagonalVeriktYAML)
		wi := runAgentExperiment(t, fmt.Sprintf("with-guide-run%d", i+1), guide+"\n\n---\n\n"+taskPrompt, hexagonalVeriktYAML)
		withoutSummary.violations = append(withoutSummary.violations, wo.ViolationsTotal)
		withoutSummary.passed = append(withoutSummary.passed, wo.Passed)
		withSummary.violations = append(withSummary.violations, wi.ViolationsTotal)
		withSummary.passed = append(withSummary.passed, wi.Passed)
	}

	t.Logf("")
	t.Logf("=== Consistency Results ===")
	t.Logf("Without guide — violations per run: %v", withoutSummary.violations)
	t.Logf("With guide    — violations per run: %v", withSummary.violations)
	t.Logf("Without guide — passed: %v", withoutSummary.passed)
	t.Logf("With guide    — passed: %v", withSummary.passed)

	woVar := variance(withoutSummary.violations)
	wiVar := variance(withSummary.violations)
	t.Logf("Variance without guide: %.2f", woVar)
	t.Logf("Variance with guide:    %.2f", wiVar)
	if woVar > 0 {
		t.Logf("Variance reduction:     %.1fx", woVar/max(wiVar, 0.01))
	}
}

// saveAgentResults writes the contrast results to the experiment results dir.
func saveAgentResults(t *testing.T, without, with agentMetrics) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return
	}
	resultsDir := filepath.Join(filepath.Dir(filename), "testdata", "agent-results")
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return
	}

	// Save prompts.
	_ = os.WriteFile(filepath.Join(resultsDir, "prompt-without-guide.txt"), []byte(without.Prompt), 0644)
	_ = os.WriteFile(filepath.Join(resultsDir, "prompt-with-guide.txt"), []byte(with.Prompt), 0644)

	// Save responses.
	_ = os.WriteFile(filepath.Join(resultsDir, "response-without-guide.txt"), []byte(without.Response), 0644)
	_ = os.WriteFile(filepath.Join(resultsDir, "response-with-guide.txt"), []byte(with.Response), 0644)

	// Save metrics as JSON.
	metrics := map[string]any{
		"without_guide": metricMap(without),
		"with_guide":    metricMap(with),
		"delta": map[string]any{
			"violations_eliminated":      without.ViolationsTotal - with.ViolationsTotal,
			"compliance_gain_pct":        with.CompliancePct - without.CompliancePct,
			"guide_cache_token_overhead": with.CacheReadTokens - without.CacheReadTokens,
			"cost_delta_usd":             with.CostUSD - without.CostUSD,
			"hexagonal_shape_without":    isHexagonalShape(without.Packages),
			"hexagonal_shape_with":       isHexagonalShape(with.Packages),
			"architecture_enforced":      !isHexagonalShape(without.Packages) && isHexagonalShape(with.Packages),
		},
	}
	b, _ := json.MarshalIndent(metrics, "", "  ")
	_ = os.WriteFile(filepath.Join(resultsDir, "metrics.json"), b, 0644)

	t.Logf("Results saved to: %s", resultsDir)
}

func metricMap(m agentMetrics) map[string]any {
	return map[string]any{
		"input_tokens":      m.InputTokens,
		"cache_read_tokens": m.CacheReadTokens,
		"output_tokens":     m.OutputTokens,
		"total_tokens":      m.TotalTokens,
		"duration_ms":       m.DurationMs,
		"duration_api_ms":   m.DurationAPIMs,
		"cost_usd":          m.CostUSD,
		"violations_dep":    m.ViolationsDep,
		"violations_fn":     m.ViolationsFn,
		"violations_ap":     m.ViolationsAP,
		"violations_total":  m.ViolationsTotal,
		"compliance_pct":    m.CompliancePct,
		"passed":            m.Passed,
		"files_generated":   len(m.GeneratedFiles),
	}
}

func variance(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0
	for _, v := range vals {
		sum += v
	}
	mean := float64(sum) / float64(len(vals))
	var sq float64
	for _, v := range vals {
		d := float64(v) - mean
		sq += d * d
	}
	return sq / float64(len(vals))
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
