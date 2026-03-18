package engineclient

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/dcsg/archway/internal/engineclient/pb"
)

func TestPing(t *testing.T) {
	client := newTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := client.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	if result.Version != "0.1.0" {
		t.Errorf("expected version 0.1.0, got %s", result.Version)
	}

	if len(result.Capabilities) < 2 {
		t.Errorf("expected at least 2 capabilities, got %v", result.Capabilities)
	}
}

func TestCheckGrepFindsViolation(t *testing.T) {
	client := newTestClient(t)

	// Create a temp project with a file containing a known violation
	projectDir := t.TempDir()
	violatingCode := `package main

import "fmt"

var GlobalState = map[string]string{} // mutable global

func main() {
	fmt.Println(GlobalState)
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "main.go"), []byte(violatingCode), 0644); err != nil {
		t.Fatal(err)
	}

	rules := []*pb.Rule{
		{
			Id:       "arch/no-global-mutable-state",
			Severity: pb.Severity_ERROR,
			Message:  "Global mutable state is forbidden",
			Engine:   pb.EngineType_GREP,
			Scope: &pb.RuleScope{
				Include: []string{"*.go"},
			},
			Spec: &pb.Rule_Grep{Grep: &pb.GrepSpec{
				Pattern: `var\s+\w+\s*=\s*(map\[|&|\[\])`,
			}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := client.Check(ctx, projectDir, rules, nil)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}

	f := result.Findings[0]
	if f.RuleId != "arch/no-global-mutable-state" {
		t.Errorf("expected rule id arch/no-global-mutable-state, got %s", f.RuleId)
	}
	if f.Line != 5 {
		t.Errorf("expected line 5, got %d", f.Line)
	}
	if f.File != "main.go" {
		t.Errorf("expected file main.go, got %s", f.File)
	}
	if f.Engine != "grep" {
		t.Errorf("expected engine grep, got %s", f.Engine)
	}

	if result.Summary.FilesChecked != 1 {
		t.Errorf("expected 1 file checked, got %d", result.Summary.FilesChecked)
	}
	if result.Summary.FindingsError != 1 {
		t.Errorf("expected 1 error finding, got %d", result.Summary.FindingsError)
	}
	if result.Summary.DurationMs <= 0 {
		t.Error("expected positive duration")
	}
}

func TestCheckGrepNoViolation(t *testing.T) {
	client := newTestClient(t)

	projectDir := t.TempDir()
	cleanCode := `package main

import "fmt"

func main() {
	fmt.Println("clean code")
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "main.go"), []byte(cleanCode), 0644); err != nil {
		t.Fatal(err)
	}

	rules := []*pb.Rule{
		{
			Id:       "arch/no-global-mutable-state",
			Severity: pb.Severity_ERROR,
			Message:  "Global mutable state is forbidden",
			Engine:   pb.EngineType_GREP,
			Scope: &pb.RuleScope{
				Include: []string{"*.go"},
			},
			Spec: &pb.Rule_Grep{Grep: &pb.GrepSpec{
				Pattern: `var\s+\w+\s*=\s*(map\[|&|\[\])`,
			}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := client.Check(ctx, projectDir, rules, nil)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}

	if result.Summary.FindingsTotal != 0 {
		t.Errorf("expected 0 total findings, got %d", result.Summary.FindingsTotal)
	}

	// Rule should be STALE (matched 0 files with violations)
	if len(result.Summary.RuleStatuses) != 1 {
		t.Fatalf("expected 1 rule status, got %d", len(result.Summary.RuleStatuses))
	}
	if result.Summary.RuleStatuses[0].Status != pb.RuleStatus_STALE {
		t.Errorf("expected STALE status, got %v", result.Summary.RuleStatuses[0].Status)
	}
}

func TestCheckGrepMustNotContain(t *testing.T) {
	client := newTestClient(t)

	projectDir := t.TempDir()
	code := `package main

import "fmt"

var cache = map[string]string{} // nolint:globals
var config = map[string]string{}

func main() {
	fmt.Println(cache, config)
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "main.go"), []byte(code), 0644); err != nil {
		t.Fatal(err)
	}

	rules := []*pb.Rule{
		{
			Id:       "arch/no-global-mutable-state",
			Severity: pb.Severity_ERROR,
			Message:  "Global mutable state is forbidden",
			Engine:   pb.EngineType_GREP,
			Spec: &pb.Rule_Grep{Grep: &pb.GrepSpec{
				Pattern:        `var\s+\w+\s*=\s*map\[`,
				MustNotContain: `nolint`,
			}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := client.Check(ctx, projectDir, rules, nil)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	// Should find config (line 6) but NOT cache (line 5, has nolint)
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Line != 6 {
		t.Errorf("expected line 6, got %d", result.Findings[0].Line)
	}
}

func TestCheckGrepTargetFiles(t *testing.T) {
	client := newTestClient(t)

	projectDir := t.TempDir()
	code := `package main
var state = map[string]string{}
`
	if err := os.WriteFile(filepath.Join(projectDir, "a.go"), []byte(code), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "b.go"), []byte(code), 0644); err != nil {
		t.Fatal(err)
	}

	rules := []*pb.Rule{
		{
			Id:       "test/grep-target",
			Severity: pb.Severity_WARNING,
			Message:  "found it",
			Engine:   pb.EngineType_GREP,
			Spec:     &pb.Rule_Grep{Grep: &pb.GrepSpec{Pattern: `var state`}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Only check a.go via target_files (simulates --staged)
	result, err := client.Check(ctx, projectDir, rules, []string{filepath.Join(projectDir, "a.go")})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding (only a.go), got %d", len(result.Findings))
	}
	if result.Summary.FilesChecked != 1 {
		t.Errorf("expected 1 file checked, got %d", result.Summary.FilesChecked)
	}
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	engineBin := findEngineBinary(t)
	return New(engineBin)
}

func findEngineBinary(t *testing.T) string {
	t.Helper()

	if p := os.Getenv("ARCHWAY_ENGINE_BIN"); p != "" {
		return p
	}

	repoRoot := findRepoRoot(t)
	debugPath := filepath.Join(repoRoot, "engine", "target", "debug", "archway-engine")
	if _, err := os.Stat(debugPath); err == nil {
		return debugPath
	}

	// Fall back to the extracted cache binary before trying to build from source.
	if cacheDir, err := os.UserCacheDir(); err == nil {
		cachePath := filepath.Join(cacheDir, "archway", "engine-v"+version, "archway-engine")
		if _, err := os.Stat(cachePath); err == nil {
			return cachePath
		}
	}

	t.Log("Engine binary not found, building...")
	cmd := exec.Command("cargo", "build") //nolint:noctx
	cmd.Dir = filepath.Join(repoRoot, "engine")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build engine: %v\n%s", err, out)
	}

	return debugPath
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}
