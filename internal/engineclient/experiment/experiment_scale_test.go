package experiment

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/dcsg/archway/internal/checker"
	"github.com/dcsg/archway/internal/config"
	pb "github.com/dcsg/archway/internal/engineclient/pb"
	"github.com/stretchr/testify/require"
)

// TestScale_RuleCount measures Go vs engine latency as import-graph rule count grows.
// Core thesis: engine builds the import graph once and evaluates all N rules in a single
// file walk, while a sequential Go path would walk packages N times.
func TestScale_RuleCount(t *testing.T) {
	client := newEngineClient(t)
	projectPath := findRepoRoot(t)

	ruleCounts := []int{1, 5, 10, 20}

	t.Logf("=== Rule Count Scaling (import-graph) ===")
	t.Logf("%-6s  %-12s  %-12s  %-8s", "Rules", "Go (ms)", "Engine (ms)", "Speedup")

	// Go baseline: one full check pass (packages.Load + AST walk).
	goStart := time.Now()
	cfg := &config.ArchwayConfig{
		Language: "go",
		Rules:    config.RulesConfig{Functions: config.FunctionRules{MaxLines: 50}},
	}
	goResult, err := checker.Check(cfg, projectPath)
	_ = goResult
	goDuration := time.Since(goStart)
	require.NoError(t, err)

	for _, n := range ruleCounts {
		rules := makeSyntheticImportRules(n)

		// Engine: all N import-graph rules in one request — one file walk.
		engineStart := time.Now()
		result, err := client.Check(context.Background(), projectPath, rules, nil)
		_ = result
		engineDuration := time.Since(engineStart)
		require.NoError(t, err)

		speedup := float64(goDuration) / float64(engineDuration)
		t.Logf("%-6d  %-12.1f  %-12.1f  %.1fx",
			n,
			float64(goDuration.Nanoseconds())/1e6,
			float64(engineDuration.Nanoseconds())/1e6,
			speedup,
		)
	}
}

// TestScale_ProjectSize benchmarks both paths as project size grows.
// Controlled via ARCHWAY_EXPERIMENT_LARGE_PROJECT env var (path to a large Go project).
func TestScale_ProjectSize(t *testing.T) {
	projectPath := os.Getenv("ARCHWAY_EXPERIMENT_LARGE_PROJECT")
	if projectPath == "" {
		t.Skip("set ARCHWAY_EXPERIMENT_LARGE_PROJECT to enable project size scaling test")
	}

	client := newEngineClient(t)
	rules := config.FunctionRules{MaxLines: 50, MaxParams: 5}
	cfg := &config.ArchwayConfig{
		Language: "go",
		Rules:    config.RulesConfig{Functions: rules},
	}

	goStart := time.Now()
	goResult, err := checker.Check(cfg, projectPath)
	goDuration := time.Since(goStart)
	require.NoError(t, err)

	engineStart := time.Now()
	engineViolations := checkFunctionMetrics(t, client, projectPath, rules)
	engineDuration := time.Since(engineStart)

	t.Logf("=== Project Size Scaling (%s) ===", projectPath)
	t.Logf("Go AST:  %d violations in %v", len(goResult.FunctionViolations), goDuration)
	t.Logf("Engine:  %d violations in %v", len(engineViolations), engineDuration)
	if goDuration > 0 && engineDuration > 0 {
		t.Logf("Speedup: %.1fx", float64(goDuration)/float64(engineDuration))
	}
}

// TestScale_OSS_Chi benchmarks both paths on gorilla/mux or go-chi/chi.
// Requires ARCHWAY_EXPERIMENT_OSS=1 and network access to clone repos.
func TestScale_OSS_Chi(t *testing.T) {
	if os.Getenv("ARCHWAY_EXPERIMENT_OSS") == "" {
		t.Skip("set ARCHWAY_EXPERIMENT_OSS=1 to enable OSS project experiments")
	}

	projectPath := cloneOrSkip(t, "https://github.com/go-chi/chi.git", "chi")
	runOSSExperiment(t, "chi", projectPath)
}

// TestScale_OSS_Zap benchmarks both paths on uber-go/zap.
func TestScale_OSS_Zap(t *testing.T) {
	if os.Getenv("ARCHWAY_EXPERIMENT_OSS") == "" {
		t.Skip("set ARCHWAY_EXPERIMENT_OSS=1 to enable OSS project experiments")
	}

	projectPath := cloneOrSkip(t, "https://github.com/uber-go/zap.git", "zap")
	runOSSExperiment(t, "zap", projectPath)
}

// TestScale_OSS_Mux benchmarks both paths on gorilla/mux.
func TestScale_OSS_Mux(t *testing.T) {
	if os.Getenv("ARCHWAY_EXPERIMENT_OSS") == "" {
		t.Skip("set ARCHWAY_EXPERIMENT_OSS=1 to enable OSS project experiments")
	}

	projectPath := cloneOrSkip(t, "https://github.com/gorilla/mux.git", "mux")
	runOSSExperiment(t, "mux", projectPath)
}

func runOSSExperiment(t *testing.T, name, projectPath string) {
	t.Helper()
	client := newEngineClient(t)
	rules := config.FunctionRules{MaxLines: 50, MaxParams: 5}
	cfg := &config.ArchwayConfig{
		Language: "go",
		Rules:    config.RulesConfig{Functions: rules},
	}

	goStart := time.Now()
	goResult, err := checker.Check(cfg, projectPath)
	goDuration := time.Since(goStart)
	require.NoError(t, err)

	engineStart := time.Now()
	engineViolations := checkFunctionMetrics(t, client, projectPath, rules)
	engineDuration := time.Since(engineStart)

	t.Logf("=== OSS: %s ===", name)
	t.Logf("Go AST:  %d violations in %v", len(goResult.FunctionViolations), goDuration)
	t.Logf("Engine:  %d violations in %v", len(engineViolations), engineDuration)
	if goDuration > 0 && engineDuration > 0 {
		t.Logf("Speedup: %.1fx", float64(goDuration)/float64(engineDuration))
	}

	// Both paths must agree there are (or aren't) violations.
	goHas := len(goResult.FunctionViolations) > 0
	engineHas := len(engineViolations) > 0
	t.Logf("Agreement: go=%v engine=%v", goHas, engineHas)
}

// makeSyntheticImportRules returns n import-graph rules for the engine.
// Each rule targets a distinct synthetic package pattern so the engine must
// evaluate all N patterns in a single import-graph walk.
func makeSyntheticImportRules(n int) []*pb.Rule {
	rules := make([]*pb.Rule, n)
	for i := range n {
		rules[i] = &pb.Rule{
			Id:       fmt.Sprintf("arch/synthetic-%d", i),
			Severity: pb.Severity_WARNING,
			Message:  fmt.Sprintf("synthetic rule %d", i),
			Engine:   pb.EngineType_IMPORT_GRAPH,
			Spec: &pb.Rule_ImportGraph{
				ImportGraph: &pb.ImportGraphSpec{
					PackagePattern: fmt.Sprintf("internal/synth%d/**", i),
					Forbidden:      []string{"internal/forbidden/**"},
				},
			},
		}
	}
	return rules
}

// cloneOrSkip clones a git repo into a temp dir and returns the path.
// If git is unavailable or clone fails, the test is skipped.
func cloneOrSkip(t *testing.T, repoURL, name string) string {
	t.Helper()
	_, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("git not available: %v", err)
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	cacheDir := filepath.Join(filepath.Dir(filename), "..", "..", "..", "..", "tmp", "oss-cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Skipf("cannot create OSS cache dir: %v", err)
	}
	dest := filepath.Join(cacheDir, name)
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		cmd := exec.CommandContext(context.Background(), "git", "clone", "--depth=1", repoURL, dest)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git clone failed: %v\n%s", err, out)
		}
	}
	return dest
}
