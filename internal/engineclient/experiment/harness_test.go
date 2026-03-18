package experiment

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// RunConfig holds the parameters for a single experiment run.
type RunConfig struct {
	ExperimentID string // "EXP-04"
	Condition    string // "control" | "test" | "A-lazy-noguide" etc.
	Run          int    // 1, 2, or 3
	Label        string // for log output, e.g. "EXP10-control-run1"

	// Mode A (greenfield): ArchwayYAML is used to write the project and generate the guide.
	ArchwayYAML string
	TaskPrompt  string
	WithGuide   bool

	// Mode B (feature-addition): FixtureName names a dir under testdata/.
	FixtureName string

	// FixtureInUserPrompt moves the fixture block from the system prompt into the
	// user prompt. The system prompt becomes a minimal role + file-format instruction
	// without the "no explanations outside file blocks" restriction. Use this when
	// the task asks the agent to produce content after the files (e.g. recommendations).
	FixtureInUserPrompt bool
}

// ExperimentRun is the result of one condition run, returned to the test for assertions.
type ExperimentRun struct {
	agentMetrics
	ArtifactDir string
}

// RunGreenfield runs a Mode A experiment: no fixture, agent builds from scratch.
// The agent receives the system prompt and task prompt (with optional guide prepended).
// archway check runs on the full generated project.
func RunGreenfield(t *testing.T, cfg RunConfig) ExperimentRun {
	t.Helper()
	agentGuardOrSkip(t)

	agent := agentFromEnvOrFail(t)
	bin := archwayBin(t)

	systemPrompt := greenfieldSystemPrompt
	userPrompt := cfg.TaskPrompt
	var guidePtr *string
	if cfg.WithGuide {
		guide := generateGuideFromYAML(t, bin, cfg.ArchwayYAML)
		userPrompt = guide + "\n\n---\n\n" + cfg.TaskPrompt
		guidePtr = &guide
	}

	t.Logf("[%s] calling %s (greenfield)...", cfg.Label, agent.ID())
	resp, err := agent.Call(context.Background(), systemPrompt, userPrompt)
	if err != nil {
		t.Fatalf("[%s] agent call failed: %v", cfg.Label, err)
	}
	t.Logf("[%s] done in %dms — input=%d cache=%d output=%d",
		cfg.Label, resp.DurationMS, resp.InputTokens, resp.CacheTokens, resp.OutputTokens)

	files := parseGeneratedFiles(resp.Raw)
	t.Logf("[%s] generated %d files", cfg.Label, len(files))

	projectDir := writeProject(t, files, cfg.ArchwayYAML)
	dep, fn, ap, arch, compliance, passed := runArchwayCheck(t, bin, projectDir)
	total := dep + fn + ap + arch
	t.Logf("[%s] archway check: dep=%d fn=%d ap=%d arch=%d total=%d passed=%v",
		cfg.Label, dep, fn, ap, arch, total, passed)

	checkJSON := rawArchwayCheckJSON(t, bin, projectDir, false)
	pkgs := detectPackages(files)

	metrics := runMetrics{
		ViolationsDep:   dep,
		ViolationsFn:    fn,
		ViolationsAP:    ap,
		ViolationsArch:  arch,
		ViolationsTotal: total,
		Passed:          passed,
		HexagonalShape:  isHexagonalShape(pkgs),
		Packages:        pkgs,
		FilesGenerated:  len(files),
		InputTokens:     resp.InputTokens,
		CacheTokens:     resp.CacheTokens,
		OutputTokens:    resp.OutputTokens,
		DurationMS:      resp.DurationMS,
	}

	mf := runManifest{
		ExperimentID:  cfg.ExperimentID,
		Condition:     cfg.Condition,
		Run:           cfg.Run,
		Date:          time.Now().UTC().Format(time.RFC3339),
		Agent:         agentRunMeta{ID: agent.ID()},
		SystemPrompt:  systemPrompt,
		Guide:         guidePtr,
		TaskPrompt:    cfg.TaskPrompt,
		FullPromptSHA: promptSHA(systemPrompt, userPrompt),
	}

	aid := safeAgentID(agent)
	dir := experimentArtifactDir(cfg.ExperimentID, aid, cfg.Condition, cfg.Run)
	if persistErr := persistArtifacts(dir, runArtifacts{
		Manifest:       mf,
		Response:       resp.Raw,
		GeneratedFiles: files,
		CheckResult:    checkJSON,
		Metrics:        metrics,
	}); persistErr != nil {
		t.Logf("[%s] warning: failed to persist artifacts: %v", cfg.Label, persistErr)
	}
	updateExperimentIndex(cfg.ExperimentID, aid)

	_ = compliance
	return ExperimentRun{
		agentMetrics: agentMetrics{
			Label:           cfg.Label,
			Response:        resp.Raw,
			InputTokens:     resp.InputTokens,
			CacheReadTokens: resp.CacheTokens,
			OutputTokens:    resp.OutputTokens,
			DurationMs:      resp.DurationMS,
			DurationAPIMs:   resp.DurationMS,
			ViolationsDep:   dep,
			ViolationsFn:    fn,
			ViolationsAP:    ap,
			ViolationsArch:  arch,
			ViolationsTotal: total,
			CompliancePct:   compliance * 100,
			Passed:          passed,
			GeneratedFiles:  files,
			Packages:        pkgs,
		},
		ArtifactDir: dir,
	}
}

// RunFeatureAddition runs a Mode B experiment: fixture embedded in the system prompt.
// The agent sees the full existing codebase as text and returns only changed/new files.
// archway check --diff HEAD measures only violations in the agent's changed files.
func RunFeatureAddition(t *testing.T, cfg RunConfig) ExperimentRun {
	t.Helper()
	agentGuardOrSkip(t)

	agent := agentFromEnvOrFail(t)
	bin := archwayBin(t)

	fixtureSrcDir := testdataPath(t, cfg.FixtureName)

	fixtureBlock, fixtureSHA, err := embedFixture(fixtureSrcDir)
	if err != nil {
		t.Fatalf("[%s] embed fixture: %v", cfg.Label, err)
	}

	// Copy fixture to temp dir for archway guide and check --diff.
	workDir := t.TempDir()
	copyDir(t, fixtureSrcDir, workDir)
	gitInit(t, workDir)

	var systemPrompt, userPrompt string
	var guidePtr *string

	if cfg.FixtureInUserPrompt {
		// Fixture goes in user prompt — system prompt stays open (no "no explanations" restriction).
		systemPrompt = openFixtureSystemPrompt
		userPrompt = "Here is the current codebase:\n" + fixtureBlock + "\n---\n\n" + cfg.TaskPrompt
		if cfg.WithGuide {
			guide := generateGuide(t, bin, workDir)
			userPrompt = guide + "\n\n---\n\n" + userPrompt
			guidePtr = &guide
		}
	} else {
		// Default: fixture embedded in system prompt.
		systemPrompt = embeddedFixtureSystemPrompt + "\n\nCurrent codebase:\n" + fixtureBlock
		userPrompt = cfg.TaskPrompt
		if cfg.WithGuide {
			guide := generateGuide(t, bin, workDir)
			userPrompt = guide + "\n\n---\n\n" + cfg.TaskPrompt
			guidePtr = &guide
		}
	}

	t.Logf("[%s] calling %s (feature-addition, fixture=%s)...", cfg.Label, agent.ID(), cfg.FixtureName)
	resp, err := agent.Call(context.Background(), systemPrompt, userPrompt)
	if err != nil {
		t.Fatalf("[%s] agent call failed: %v", cfg.Label, err)
	}
	t.Logf("[%s] done in %dms — input=%d cache=%d output=%d",
		cfg.Label, resp.DurationMS, resp.InputTokens, resp.CacheTokens, resp.OutputTokens)

	files := parseGeneratedFiles(resp.Raw)
	t.Logf("[%s] generated %d files", cfg.Label, len(files))

	// Write agent output on top of the fixture in workDir.
	for path, content := range files {
		full := filepath.Join(workDir, path)
		if mkErr := os.MkdirAll(filepath.Dir(full), 0755); mkErr != nil {
			t.Logf("[%s] mkdir %s: %v", cfg.Label, path, mkErr)
			continue
		}
		if wErr := os.WriteFile(full, []byte(content), 0644); wErr != nil {
			t.Logf("[%s] write %s: %v", cfg.Label, path, wErr)
		}
	}

	// Stage agent changes so --diff HEAD sees them.
	gitAddAllFiles(t, workDir)

	dep, fn, ap, arch, compliance, passed := runArchwayCheckDiff(t, bin, workDir)
	total := dep + fn + ap + arch
	t.Logf("[%s] archway check --diff HEAD: dep=%d fn=%d ap=%d arch=%d total=%d passed=%v",
		cfg.Label, dep, fn, ap, arch, total, passed)

	checkDiffJSON := rawArchwayCheckJSON(t, bin, workDir, true)
	pkgs := detectPackages(files)

	metrics := runMetrics{
		ViolationsDep:   dep,
		ViolationsFn:    fn,
		ViolationsAP:    ap,
		ViolationsArch:  arch,
		ViolationsTotal: total,
		Passed:          passed,
		HexagonalShape:  isHexagonalShape(pkgs),
		Packages:        pkgs,
		FilesGenerated:  len(files),
		InputTokens:     resp.InputTokens,
		CacheTokens:     resp.CacheTokens,
		OutputTokens:    resp.OutputTokens,
		DurationMS:      resp.DurationMS,
	}

	mf := runManifest{
		ExperimentID:  cfg.ExperimentID,
		Condition:     cfg.Condition,
		Run:           cfg.Run,
		Date:          time.Now().UTC().Format(time.RFC3339),
		Agent:         agentRunMeta{ID: agent.ID()},
		Fixture:       &fixtureMeta{Path: cfg.FixtureName, SHA256: fixtureSHA},
		SystemPrompt:  systemPrompt,
		Guide:         guidePtr,
		TaskPrompt:    cfg.TaskPrompt,
		FullPromptSHA: promptSHA(systemPrompt, userPrompt),
	}

	aid := safeAgentID(agent)
	artifDir := experimentArtifactDir(cfg.ExperimentID, aid, cfg.Condition, cfg.Run)
	if persistErr := persistArtifacts(artifDir, runArtifacts{
		Manifest:       mf,
		Response:       resp.Raw,
		GeneratedFiles: files,
		CheckDiff:      checkDiffJSON,
		Metrics:        metrics,
	}); persistErr != nil {
		t.Logf("[%s] warning: failed to persist artifacts: %v", cfg.Label, persistErr)
	}
	updateExperimentIndex(cfg.ExperimentID, aid)

	_ = compliance
	return ExperimentRun{
		agentMetrics: agentMetrics{
			Label:           cfg.Label,
			Response:        resp.Raw,
			InputTokens:     resp.InputTokens,
			CacheReadTokens: resp.CacheTokens,
			OutputTokens:    resp.OutputTokens,
			DurationMs:      resp.DurationMS,
			DurationAPIMs:   resp.DurationMS,
			ViolationsDep:   dep,
			ViolationsFn:    fn,
			ViolationsAP:    ap,
			ViolationsArch:  arch,
			ViolationsTotal: total,
			CompliancePct:   compliance * 100,
			Passed:          passed,
			GeneratedFiles:  files,
			Packages:        pkgs,
		},
		ArtifactDir: artifDir,
	}
}

// agentFromEnvOrFail constructs an Agent from env or fails the test immediately.
func agentFromEnvOrFail(t *testing.T) Agent {
	t.Helper()
	a, err := AgentFromEnv()
	if err != nil {
		t.Fatalf("configure agent: %v", err)
	}
	return a
}

// gitAddAllFiles stages all changes in dir.
func gitAddAllFiles(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "add", "-A")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add -A: %v\n%s", err, out)
	}
}

// rawArchwayCheckJSON runs archway check and returns the raw JSON output.
func rawArchwayCheckJSON(t *testing.T, bin, dir string, diff bool) json.RawMessage {
	t.Helper()
	var cmd *exec.Cmd
	if diff {
		cmd = exec.CommandContext(context.Background(), bin, "check", "--diff", "HEAD", "-o", "json")
	} else {
		cmd = exec.CommandContext(context.Background(), bin, "check", "-o", "json")
	}
	cmd.Dir = dir
	out, _ := cmd.Output()
	if len(out) == 0 {
		return nil
	}
	return json.RawMessage(out)
}
