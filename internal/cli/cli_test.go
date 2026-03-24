package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/diktahq/verikt/providers/golang"
)

func executeCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
}

func TestVersionCommand(t *testing.T) {
	_, err := executeCommand(t, "version")
	if err != nil {
		t.Fatalf("version command should not error, got: %v", err)
	}
}

func TestInvalidOutputFlag(t *testing.T) {
	_, err := executeCommand(t, "--output", "invalid", "version")
	if err == nil {
		t.Fatal("expected error for invalid --output flag")
	}
	if !strings.Contains(err.Error(), "invalid --output value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewHexagonalWithHTTPAPI(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	_, err := executeCommand(t,
		"new", "test-svc",
		"--arch", "hexagonal",
		"--cap", "http-api",
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err != nil {
		t.Fatalf("new command failed: %v", err)
	}

	svcDir := filepath.Join(tmp, "test-svc")

	// Verify output directory exists.
	if _, err := os.Stat(svcDir); os.IsNotExist(err) {
		t.Fatal("expected service directory to exist")
	}

	// Verify key files and dirs exist.
	for _, p := range []string{
		"go.mod",
		"verikt.yaml",
		"domain",
		"adapter",
		"port",
		"service",
	} {
		full := filepath.Join(svcDir, p)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", p)
		}
	}

	// Verify verikt.yaml contains architecture: hexagonal.
	data, err := os.ReadFile(filepath.Join(svcDir, "verikt.yaml"))
	if err != nil {
		t.Fatalf("failed to read verikt.yaml: %v", err)
	}
	if !strings.Contains(string(data), "architecture: hexagonal") {
		t.Errorf("verikt.yaml should contain 'architecture: hexagonal', got:\n%s", string(data))
	}
}

func TestNewHexagonalWithBFF(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	_, err := executeCommand(t,
		"new", "test-bff",
		"--arch", "hexagonal",
		"--cap", "http-api,bff,health",
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err != nil {
		t.Fatalf("new command failed: %v", err)
	}

	svcDir := filepath.Join(tmp, "test-bff")

	// Verify BFF gateway files exist.
	for _, p := range []string{
		"adapter/bffgateway/gateway.go",
		"adapter/bffgateway/httpclient.go",
		"adapter/httphandler/handler.go",
		"verikt.yaml",
	} {
		full := filepath.Join(svcDir, p)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", p)
		}
	}

	// Verify verikt.yaml includes bff capability.
	data, err := os.ReadFile(filepath.Join(svcDir, "verikt.yaml"))
	if err != nil {
		t.Fatalf("failed to read verikt.yaml: %v", err)
	}
	if !strings.Contains(string(data), "bff") {
		t.Errorf("verikt.yaml should list bff capability")
	}
}

func TestNewFlat(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	_, err := executeCommand(t,
		"new", "test-flat",
		"--arch", "flat",
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err != nil {
		t.Fatalf("new command failed: %v", err)
	}

	svcDir := filepath.Join(tmp, "test-flat")

	for _, p := range []string{
		"main.go",
		"go.mod",
		"verikt.yaml",
	} {
		full := filepath.Join(svcDir, p)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", p)
		}
	}

	data, err := os.ReadFile(filepath.Join(svcDir, "verikt.yaml"))
	if err != nil {
		t.Fatalf("failed to read verikt.yaml: %v", err)
	}
	if !strings.Contains(string(data), "architecture: flat") {
		t.Errorf("verikt.yaml should contain 'architecture: flat', got:\n%s", string(data))
	}
}

func TestNewLayered(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	_, err := executeCommand(t,
		"new", "test-layered",
		"--arch", "layered",
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err != nil {
		t.Fatalf("new command failed: %v", err)
	}

	svcDir := filepath.Join(tmp, "test-layered")

	for _, p := range []string{
		"go.mod",
		"verikt.yaml",
		filepath.Join("internal", "handler"),
		filepath.Join("internal", "service"),
		filepath.Join("internal", "repository"),
		filepath.Join("internal", "model"),
	} {
		full := filepath.Join(svcDir, p)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", p)
		}
	}

	data, err := os.ReadFile(filepath.Join(svcDir, "verikt.yaml"))
	if err != nil {
		t.Fatalf("failed to read verikt.yaml: %v", err)
	}
	if !strings.Contains(string(data), "architecture: layered") {
		t.Errorf("verikt.yaml should contain 'architecture: layered', got:\n%s", string(data))
	}
}

func TestNewClean(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	_, err := executeCommand(t,
		"new", "test-clean",
		"--arch", "clean",
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err != nil {
		t.Fatalf("new command failed: %v", err)
	}

	svcDir := filepath.Join(tmp, "test-clean")

	for _, p := range []string{
		"go.mod",
		"verikt.yaml",
		filepath.Join("internal", "entity"),
		filepath.Join("internal", "usecase"),
		filepath.Join("internal", "interface", "handler"),
		filepath.Join("internal", "infrastructure", "config"),
	} {
		full := filepath.Join(svcDir, p)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", p)
		}
	}

	data, err := os.ReadFile(filepath.Join(svcDir, "verikt.yaml"))
	if err != nil {
		t.Fatalf("failed to read verikt.yaml: %v", err)
	}
	if !strings.Contains(string(data), "architecture: clean") {
		t.Errorf("verikt.yaml should contain 'architecture: clean', got:\n%s", string(data))
	}
}

func scaffoldClean(t *testing.T, dir, name string) string {
	t.Helper()
	chdir(t, dir)
	_, err := executeCommand(t,
		"new", name,
		"--arch", "clean",
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err != nil {
		t.Fatalf("scaffold clean failed: %v", err)
	}
	return filepath.Join(dir, name)
}

func TestGuide_CleanArchitecture(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldClean(t, tmp, "guide-clean")

	chdir(t, svcDir)
	_, err := executeCommand(t, "guide", "--target", "claude")
	if err != nil {
		t.Fatalf("guide command failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(svcDir, ".claude", "rules", "verikt-index.md"))
	if err != nil {
		t.Fatalf("failed to read verikt-index.md: %v", err)
	}

	content := string(data)
	for _, want := range []string{"clean", "entity", "usecase"} {
		if !strings.Contains(content, want) {
			t.Errorf("guide index content should contain %q", want)
		}
	}
	if !strings.Contains(content, "## Layer Rules") {
		t.Errorf("guide index should contain layer rules")
	}
}

func scaffoldLayered(t *testing.T, dir, name string) string {
	t.Helper()
	chdir(t, dir)
	_, err := executeCommand(t,
		"new", name,
		"--arch", "layered",
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err != nil {
		t.Fatalf("scaffold layered failed: %v", err)
	}
	return filepath.Join(dir, name)
}

func TestGuide_LayeredArchitecture(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldLayered(t, tmp, "guide-layered")

	chdir(t, svcDir)
	_, err := executeCommand(t, "guide", "--target", "claude")
	if err != nil {
		t.Fatalf("guide command failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(svcDir, ".claude", "rules", "verikt-index.md"))
	if err != nil {
		t.Fatalf("failed to read verikt-index.md: %v", err)
	}

	content := string(data)
	for _, want := range []string{"layered", "handler", "service", "repository", "model"} {
		if !strings.Contains(content, want) {
			t.Errorf("guide index content should contain %q", want)
		}
	}
	if !strings.Contains(content, "## Layer Rules") {
		t.Errorf("guide index should contain layer rules")
	}
}

func TestNewInvalidArchitecture(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	_, err := executeCommand(t,
		"new", "test-bad",
		"--arch", "nonexistent",
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err == nil {
		t.Fatal("expected error for invalid architecture")
	}
}

func TestNewMultipleCapabilities(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	_, err := executeCommand(t,
		"new", "test-multi",
		"--arch", "hexagonal",
		"--cap", "http-api,mysql,docker",
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err != nil {
		t.Fatalf("new command failed: %v", err)
	}

	svcDir := filepath.Join(tmp, "test-multi")
	data, err := os.ReadFile(filepath.Join(svcDir, "verikt.yaml"))
	if err != nil {
		t.Fatalf("failed to read verikt.yaml: %v", err)
	}

	content := string(data)
	for _, cap := range []string{"http-api", "mysql", "docker"} {
		if !strings.Contains(content, cap) {
			t.Errorf("verikt.yaml should list capability %q, got:\n%s", cap, content)
		}
	}
}

func TestNewRequiresNameWithNoWizard(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	_, err := executeCommand(t,
		"new",
		"--arch", "hexagonal",
		"--no-wizard",
	)
	if err == nil {
		t.Fatal("expected error when name is missing with --no-wizard")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func scaffoldHexagonal(t *testing.T, dir, name string) string {
	t.Helper()
	chdir(t, dir)
	_, err := executeCommand(t,
		"new", name,
		"--arch", "hexagonal",
		"--cap", "http-api",
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err != nil {
		t.Fatalf("scaffold hexagonal failed: %v", err)
	}
	return filepath.Join(dir, name)
}

func scaffoldFlat(t *testing.T, dir, name string) string {
	t.Helper()
	chdir(t, dir)
	_, err := executeCommand(t,
		"new", name,
		"--arch", "flat",
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err != nil {
		t.Fatalf("scaffold flat failed: %v", err)
	}
	return filepath.Join(dir, name)
}

func TestGuide_GeneratesAllTargets(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldHexagonal(t, tmp, "guide-all")

	chdir(t, svcDir)
	_, err := executeCommand(t, "guide")
	if err != nil {
		t.Fatalf("guide command failed: %v", err)
	}

	for _, p := range []string{
		filepath.Join(".claude", "rules", "verikt-index.md"),
		".cursorrules",
		filepath.Join(".github", "copilot-instructions.md"),
		".windsurfrules",
	} {
		full := filepath.Join(svcDir, p)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", p)
		}
	}
}

func TestGuide_SingleTarget(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldHexagonal(t, tmp, "guide-single")

	// Remove all guide files created by scaffold so we can verify single-target behavior.
	rulesDir := filepath.Join(svcDir, ".claude", "rules")
	entries, _ := os.ReadDir(rulesDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "verikt-") {
			_ = os.Remove(filepath.Join(rulesDir, e.Name()))
		}
	}
	for _, p := range []string{
		".cursorrules",
		".windsurfrules",
		filepath.Join(".github", "copilot-instructions.md"),
	} {
		_ = os.Remove(filepath.Join(svcDir, p))
	}

	chdir(t, svcDir)
	_, err := executeCommand(t, "guide", "--target", "claude")
	if err != nil {
		t.Fatalf("guide --target claude failed: %v", err)
	}

	// Claude target should exist (split files).
	claudePath := filepath.Join(svcDir, ".claude", "rules", "verikt-index.md")
	if _, err := os.Stat(claudePath); os.IsNotExist(err) {
		t.Error("expected .claude/rules/verikt-index.md to exist")
	}

	// Other targets should NOT exist.
	for _, p := range []string{".cursorrules", ".windsurfrules", filepath.Join(".github", "copilot-instructions.md")} {
		full := filepath.Join(svcDir, p)
		if _, err := os.Stat(full); err == nil {
			t.Errorf("expected %s to NOT exist for single target", p)
		}
	}
}

func TestGuide_InvalidTarget(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldHexagonal(t, tmp, "guide-invalid")

	chdir(t, svcDir)
	_, err := executeCommand(t, "guide", "--target", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
}

func TestGuide_NoVeriktYAML(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	_, err := executeCommand(t, "guide")
	if err == nil {
		t.Fatal("expected error when no verikt.yaml exists")
	}
	if !strings.Contains(err.Error(), "verikt.yaml") {
		t.Fatalf("expected error about verikt.yaml, got: %v", err)
	}
}

func TestGuide_ContentContainsArchitecture(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldHexagonal(t, tmp, "guide-content")

	chdir(t, svcDir)
	_, err := executeCommand(t, "guide", "--target", "claude")
	if err != nil {
		t.Fatalf("guide command failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(svcDir, ".claude", "rules", "verikt-index.md"))
	if err != nil {
		t.Fatalf("failed to read verikt-index.md: %v", err)
	}

	content := string(data)
	for _, want := range []string{"hexagonal", "Layer Rules", "Active Capabilities"} {
		if !strings.Contains(content, want) {
			t.Errorf("guide content should contain %q", want)
		}
	}
}

func TestGuide_FlatArchitecture(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldFlat(t, tmp, "guide-flat")

	chdir(t, svcDir)
	_, err := executeCommand(t, "guide", "--target", "claude")
	if err != nil {
		t.Fatalf("guide command failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(svcDir, ".claude", "rules", "verikt-index.md"))
	if err != nil {
		t.Fatalf("failed to read verikt-index.md: %v", err)
	}

	content := string(data)
	for _, want := range []string{"flat", "no layer restrictions"} {
		if !strings.Contains(content, want) {
			t.Errorf("guide content should contain %q", want)
		}
	}
}

// --- Check command tests ---

func TestCheck_CleanHexagonalProject(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldHexagonal(t, tmp, "check-hex")

	_, err := executeCommand(t, "check", "--path", svcDir)
	if err != nil {
		t.Fatalf("check should pass on clean hexagonal project: %v", err)
	}
}

func TestCheck_NoVeriktYAML(t *testing.T) {
	tmp := t.TempDir()

	_, err := executeCommand(t, "check", "--path", tmp)
	if err == nil {
		t.Fatal("expected error when no verikt.yaml exists")
	}
	if !strings.Contains(err.Error(), "verikt.yaml") {
		t.Fatalf("expected error about verikt.yaml, got: %v", err)
	}
}

func TestCheck_FlatProject(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldFlat(t, tmp, "check-flat")

	_, err := executeCommand(t, "check", "--path", svcDir)
	if err != nil {
		t.Fatalf("check should pass on clean flat project: %v", err)
	}
}

// --- Analyze command tests ---

func TestAnalyze_HexagonalProject(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldHexagonal(t, tmp, "analyze-hex")

	_, err := executeCommand(t, "analyze", "--path", svcDir)
	if err != nil {
		t.Fatalf("analyze should succeed on hexagonal project: %v", err)
	}
}

func TestAnalyze_EmptyDir(t *testing.T) {
	tmp := t.TempDir()

	_, err := executeCommand(t, "analyze", "--path", tmp)
	if err == nil {
		t.Fatal("expected error when analyzing empty directory")
	}
}

func TestAnalyze_JsonOutput(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldHexagonal(t, tmp, "analyze-json")

	// Capture stdout to verify JSON output.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	_, execErr := executeCommand(t, "analyze", "--path", svcDir, "--output", "json")

	_ = w.Close()
	os.Stdout = oldStdout

	if execErr != nil {
		t.Fatalf("analyze --output json failed: %v", execErr)
	}

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// The output may contain a "Detected language:" prefix line before JSON.
	// Find the first '{' to extract the JSON object.
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("expected JSON object in output, got:\n%s", output)
	}
	jsonStr := output[idx:]
	if !json.Valid([]byte(jsonStr)) {
		t.Fatalf("expected valid JSON output, got:\n%s", jsonStr)
	}
}

// --- Add command tests ---

func TestAddCapabilityToExistingProject(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldHexagonal(t, tmp, "add-redis")

	chdir(t, svcDir)
	_, err := executeCommand(t, "add", "redis")
	if err != nil {
		t.Fatalf("add redis failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(svcDir, "verikt.yaml"))
	if err != nil {
		t.Fatalf("failed to read verikt.yaml: %v", err)
	}
	if !strings.Contains(string(data), "redis") {
		t.Errorf("verikt.yaml should contain 'redis' after add, got:\n%s", string(data))
	}
}

func TestAddAlreadyInstalledCapability(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldHexagonal(t, tmp, "add-dup")

	chdir(t, svcDir)

	// Capture stdout since add prints via fmt.Printf.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	_, execErr := executeCommand(t, "add", "http-api")

	w.Close()
	os.Stdout = oldStdout

	if execErr != nil {
		t.Fatalf("add http-api should not error: %v", execErr)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	output := buf.String()
	if !strings.Contains(output, "already installed") {
		t.Errorf("expected output to contain 'already installed', got: %s", output)
	}
}

func TestAddUnknownCapability(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldHexagonal(t, tmp, "add-unknown")

	chdir(t, svcDir)
	_, err := executeCommand(t, "add", "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown capability")
	}
	if !strings.Contains(err.Error(), "unknown capability") {
		t.Fatalf("expected 'unknown capability' error, got: %v", err)
	}
}

func TestAddWithoutVeriktYaml(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	_, err := executeCommand(t, "add", "redis")
	if err == nil {
		t.Fatal("expected error when no verikt.yaml exists")
	}
	if !strings.Contains(err.Error(), "no verikt.yaml found") {
		t.Fatalf("expected 'no verikt.yaml found' error, got: %v", err)
	}
}

func TestAddCapabilityWithTransitiveDeps(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	// Scaffold a project with no capabilities.
	_, err := executeCommand(t,
		"new", "add-transitive",
		"--arch", "hexagonal",
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err != nil {
		t.Fatalf("new command failed: %v", err)
	}

	svcDir := filepath.Join(tmp, "add-transitive")
	chdir(t, svcDir)

	// bff requires http-api, so adding bff should also add http-api.
	_, err = executeCommand(t, "add", "bff")
	if err != nil {
		t.Fatalf("add bff failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(svcDir, "verikt.yaml"))
	if err != nil {
		t.Fatalf("failed to read verikt.yaml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "bff") {
		t.Errorf("verikt.yaml should contain 'bff', got:\n%s", content)
	}
	if !strings.Contains(content, "http-api") {
		t.Errorf("verikt.yaml should contain 'http-api' (transitive dep of bff), got:\n%s", content)
	}
}

// --- Init command tests ---

func TestInit_CreatesVeriktYAML(t *testing.T) {
	tmp := t.TempDir()

	// Create a minimal Go project.
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := executeCommand(t, "init", "--path", tmp, "--no-wizard")
	if err != nil {
		t.Fatalf("init should succeed: %v", err)
	}

	veriktPath := filepath.Join(tmp, "verikt.yaml")
	if _, err := os.Stat(veriktPath); os.IsNotExist(err) {
		t.Fatal("expected verikt.yaml to be created")
	}

	data, err := os.ReadFile(veriktPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "architecture:") {
		t.Errorf("verikt.yaml should contain architecture field, got:\n%s", string(data))
	}
}

func TestInit_ExistingVeriktYAML(t *testing.T) {
	tmp := t.TempDir()

	// Create an existing verikt.yaml.
	veriktPath := filepath.Join(tmp, "verikt.yaml")
	if err := os.WriteFile(veriktPath, []byte("architecture: flat\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := executeCommand(t, "init", "--path", tmp, "--no-wizard")
	if err == nil {
		t.Fatal("expected error when verikt.yaml already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected 'already exists' error, got: %v", err)
	}

	// With --force, it should succeed.
	_, err = executeCommand(t, "init", "--path", tmp, "--no-wizard", "--force")
	if err != nil {
		t.Fatalf("init --force should succeed: %v", err)
	}
}

// --- Diff command tests ---

func TestDiffCleanProject(t *testing.T) {
	tmp := t.TempDir()
	svcDir := scaffoldHexagonal(t, tmp, "diff-clean")

	// Capture stdout since diff prints via fmt.Printf.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	_, execErr := executeCommand(t, "diff", "--path", svcDir)

	w.Close()
	os.Stdout = oldStdout

	if execErr != nil {
		t.Fatalf("diff should pass on clean project: %v", execErr)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "drift score: 0.00") {
		t.Errorf("expected drift score 0.00 in output, got:\n%s", out)
	}
}
