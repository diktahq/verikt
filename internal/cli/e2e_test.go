package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/diktahq/verikt/providers/golang"
)

// requireGoToolchain skips the test when no go binary is on PATH. It is the
// only environmental reason the e2e tests skip: once the toolchain is present,
// a failing go mod tidy, check, or analyze is a real failure.
func requireGoToolchain(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain not found on PATH: %v", err)
	}
}

// goModTidy resolves the scaffolded project's dependencies. It needs network
// access (or a warm module cache) and fails the test when tidy fails, because
// a broken import in a template looks exactly like that.
func goModTidy(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "go", "mod", "tidy")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy in %s failed: %v\n%s", dir, err, out)
	}
}

// runCommandInDir runs a verikt command in dir and fails the test with
// everything it printed when it returns an error. check and analyze write
// their reports to os.Stdout, so stdout is captured alongside cobra's buffer.
func runCommandInDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	chdir(t, dir)
	var cobraOut string
	var err error
	stdout := captureStdout(t, func() {
		cobraOut, err = executeCommand(t, args...)
	})
	if err != nil {
		t.Fatalf("verikt %s failed: %v\n%s%s", strings.Join(args, " "), err, stdout, cobraOut)
	}
}

func TestE2E_ScaffoldCheckAnalyzeGuide(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	requireGoToolchain(t)

	tmp := t.TempDir()
	svcDir := scaffoldHexagonal(t, tmp, "e2e-hex")

	// go mod tidy so check/analyze can parse Go code.
	goModTidy(t, svcDir)

	runCommandInDir(t, svcDir, "check", "--path", ".")
	runCommandInDir(t, svcDir, "analyze", "--path", ".")

	// Guide.
	chdir(t, svcDir)
	_, err := executeCommand(t, "guide")
	if err != nil {
		t.Fatalf("guide command failed: %v", err)
	}

	for _, p := range []string{
		filepath.Join(".claude", "rules", "verikt-index.md"),
		filepath.Join(".cursor", "rules", "verikt.mdc"),
		"AGENTS.md",
		filepath.Join(".github", "copilot-instructions.md"),
		".windsurfrules",
	} {
		full := filepath.Join(svcDir, p)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Errorf("expected %s to exist after guide", p)
		}
	}

	// Verify guide content references capabilities.
	data, err := os.ReadFile(filepath.Join(svcDir, ".claude", "rules", "verikt-index.md"))
	if err != nil {
		t.Fatalf("failed to read verikt-index.md: %v", err)
	}
	content := string(data)
	for _, cap := range []string{"http-api"} {
		if !strings.Contains(content, cap) {
			t.Errorf("guide content should mention capability %q", cap)
		}
	}
}

func TestE2E_FlatPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	requireGoToolchain(t)

	tmp := t.TempDir()
	svcDir := scaffoldFlat(t, tmp, "e2e-flat")

	goModTidy(t, svcDir)

	runCommandInDir(t, svcDir, "check", "--path", ".")
	runCommandInDir(t, svcDir, "analyze", "--path", ".")

	// Guide.
	chdir(t, svcDir)
	_, err := executeCommand(t, "guide")
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

func TestE2E_MultipleCapabilities(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	requireGoToolchain(t)

	tmp := t.TempDir()
	chdir(t, tmp)

	caps := "http-api,mysql,docker,observability,health"
	_, err := executeCommand(t,
		"new", "e2e-multi",
		"--arch", "hexagonal",
		"--cap", caps,
		"--no-wizard",
		"--set", "skip_hooks=true",
	)
	if err != nil {
		t.Fatalf("scaffold failed: %v", err)
	}

	svcDir := filepath.Join(tmp, "e2e-multi")
	goModTidy(t, svcDir)

	runCommandInDir(t, svcDir, "check", "--path", ".")

	// Guide.
	chdir(t, svcDir)
	_, err = executeCommand(t, "guide", "--target", "claude")
	if err != nil {
		t.Fatalf("guide command failed: %v", err)
	}

	indexData, err := os.ReadFile(filepath.Join(svcDir, ".claude", "rules", "verikt-index.md"))
	if err != nil {
		t.Fatalf("failed to read verikt-index.md: %v", err)
	}

	content := string(indexData)
	for _, cap := range []string{"http-api", "mysql", "docker", "observability", "health"} {
		if !strings.Contains(content, cap) {
			t.Errorf("guide index should mention capability %q", cap)
		}
	}
}
