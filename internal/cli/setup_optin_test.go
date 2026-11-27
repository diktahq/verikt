package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolatedHome points HOME at a fresh temp dir so no test touches the real
// home directory, and returns it. CLAUDE_CONFIG_DIR is cleared too: a profile
// alias in the ambient environment would otherwise point at a real directory.
func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(claudeConfigDirEnv, "")
	return home
}

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func runSetupArgs(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := newSetupCommand()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestSetup_CodexNotConfiguredWithoutFlag(t *testing.T) {
	home := isolatedHome(t)
	mkdirs(t, filepath.Join(home, ".codex"))

	out, err := runSetupArgs(t)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Error("Codex must not be configured just because ~/.codex exists")
	}
	if !strings.Contains(out, "No supported AI agents detected") {
		t.Errorf("expected no auto-detected agents, got:\n%s", out)
	}
}

func TestSetup_OptInHintWhenAgentDetected(t *testing.T) {
	home := isolatedHome(t)
	mkdirs(t, filepath.Join(home, ".cursor"), filepath.Join(home, ".codex"), filepath.Join(home, ".gemini", "antigravity"))

	out, err := runSetupArgs(t)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	for _, want := range []string{"verikt setup --codex", "verikt setup --antigravity"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected opt-in hint %q, got:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".gemini", "GEMINI.md")); !os.IsNotExist(err) {
		t.Error("Antigravity must not be configured without --antigravity")
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "rules", "verikt.md")); err != nil {
		t.Errorf("Cursor should still be auto-configured: %v", err)
	}
}

func TestSetup_CodexFlagWritesSectionAndKeepsUserContent(t *testing.T) {
	home := isolatedHome(t)
	mkdirs(t, filepath.Join(home, ".codex"))
	agentsFile := filepath.Join(home, ".codex", "AGENTS.md")
	user := "# My Codex rules\n\nAlways run tests.\n"
	if err := os.WriteFile(agentsFile, []byte(user), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := runSetupArgs(t, "--codex"); err != nil {
		t.Fatalf("setup --codex: %v", err)
	}

	content := readFile(t, agentsFile)
	if !strings.HasPrefix(content, user) {
		t.Errorf("user content must be preserved at the top, got:\n%s", content)
	}
	for _, want := range []string{globalMarkers.Start, globalMarkers.End, "<!-- verikt:global:v" + version + " -->"} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q in AGENTS.md", want)
		}
	}
	info, err := os.Stat(agentsFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected file mode 0600 to be preserved, got %v", info.Mode().Perm())
	}
}

func TestSetup_CodexIdempotent(t *testing.T) {
	home := isolatedHome(t)
	mkdirs(t, filepath.Join(home, ".codex"))
	agentsFile := filepath.Join(home, ".codex", "AGENTS.md")

	if _, err := runSetupArgs(t, "--codex"); err != nil {
		t.Fatal(err)
	}
	first := readFile(t, agentsFile)

	out, err := runSetupArgs(t, "--codex")
	if err != nil {
		t.Fatal(err)
	}
	if second := readFile(t, agentsFile); second != first {
		t.Error("second run must not change the file")
	}
	if !strings.Contains(out, "already current") {
		t.Errorf("expected 'already current', got:\n%s", out)
	}
}

func TestSetup_CodexUpgradeReplacesOnlySection(t *testing.T) {
	home := isolatedHome(t)
	mkdirs(t, filepath.Join(home, ".codex"))
	agentsFile := filepath.Join(home, ".codex", "AGENTS.md")
	before := "user before\n\n"
	after := "\nuser after\n"
	old := before + globalMarkers.Start + "\n<!-- verikt:global:v0.0.1 -->\nold verikt\n" + globalMarkers.End + "\n" + after
	if err := os.WriteFile(agentsFile, []byte(old), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	if _, err := runSetupArgs(t, "--codex"); err != nil {
		t.Fatal(err)
	}

	content := readFile(t, agentsFile)
	if !strings.HasPrefix(content, before) || !strings.HasSuffix(content, after) {
		t.Errorf("user content around the section must be preserved, got:\n%s", content)
	}
	if strings.Contains(content, "old verikt") {
		t.Error("old verikt section must be replaced")
	}
	if !strings.Contains(content, "<!-- verikt:global:v"+version+" -->") {
		t.Error("expected current version sentinel")
	}
}

func TestSetup_SharedFileMalformedMarkersFailClosed(t *testing.T) {
	home := isolatedHome(t)
	mkdirs(t, filepath.Join(home, ".codex"))
	agentsFile := filepath.Join(home, ".codex", "AGENTS.md")
	malformed := "user\n" + globalMarkers.Start + "\nno end marker\n"
	if err := os.WriteFile(agentsFile, []byte(malformed), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	if _, err := runSetupArgs(t, "--codex"); err == nil {
		t.Fatal("expected an error for malformed markers")
	}
	if got := readFile(t, agentsFile); got != malformed {
		t.Errorf("file must be left untouched, got:\n%s", got)
	}

	if _, err := runSetupArgs(t, "--codex", "--uninstall"); err == nil {
		t.Fatal("expected uninstall to fail closed on malformed markers")
	}
	if got := readFile(t, agentsFile); got != malformed {
		t.Errorf("file must be left untouched by uninstall, got:\n%s", got)
	}
}

func TestSetup_AntigravityFlagWritesGeminiFile(t *testing.T) {
	home := isolatedHome(t)
	mkdirs(t, filepath.Join(home, ".gemini", "antigravity"))

	if _, err := runSetupArgs(t, "--antigravity"); err != nil {
		t.Fatalf("setup --antigravity: %v", err)
	}

	content := readFile(t, filepath.Join(home, ".gemini", "GEMINI.md"))
	if !strings.Contains(content, globalMarkers.Start) || !strings.Contains(content, "verikt") {
		t.Errorf("expected verikt section in GEMINI.md, got:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
		t.Error("Codex config dir must never be created")
	}
}

func TestSetup_OptInAgentMissingIsSkipped(t *testing.T) {
	home := isolatedHome(t)

	out, err := runSetupArgs(t, "--codex", "--antigravity")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, want := range []string{"Codex", "Antigravity", "not found"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
	for _, dir := range []string{".codex", ".gemini"} {
		if _, err := os.Stat(filepath.Join(home, dir)); !os.IsNotExist(err) {
			t.Errorf("verikt must not create ~/%s", dir)
		}
	}
}

func TestSetup_SharedUninstallKeepsUserContent(t *testing.T) {
	home := isolatedHome(t)
	mkdirs(t, filepath.Join(home, ".codex"))
	agentsFile := filepath.Join(home, ".codex", "AGENTS.md")
	user := "# Mine\n\nkeep this\n"
	if err := os.WriteFile(agentsFile, []byte(user), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	if _, err := runSetupArgs(t, "--codex"); err != nil {
		t.Fatal(err)
	}
	out, err := runSetupArgs(t, "--codex", "--uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if got := readFile(t, agentsFile); got != user {
		t.Errorf("uninstall must restore the user's file exactly, got:\n%q\nwant:\n%q", got, user)
	}
	if !strings.Contains(out, "removed verikt section") {
		t.Errorf("expected removal message, got:\n%s", out)
	}
}

func TestSetup_SharedUninstallDeletesFileVeriktCreated(t *testing.T) {
	home := isolatedHome(t)
	mkdirs(t, filepath.Join(home, ".codex"))
	agentsFile := filepath.Join(home, ".codex", "AGENTS.md")

	if _, err := runSetupArgs(t, "--codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := runSetupArgs(t, "--codex", "--uninstall"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(agentsFile); !os.IsNotExist(err) {
		t.Error("a file holding only verikt's section should be removed on uninstall")
	}
}

func TestSetup_SharedCheckMode(t *testing.T) {
	home := isolatedHome(t)
	mkdirs(t, filepath.Join(home, ".codex"))
	agentsFile := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.WriteFile(agentsFile, []byte("user only\n"), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	out, err := runSetupArgs(t, "--codex", "--check")
	if err == nil || !strings.Contains(out, "not installed") {
		t.Errorf("expected 'not installed' and an error before setup, got err=%v out:\n%s", err, out)
	}

	if _, err := runSetupArgs(t, "--codex"); err != nil {
		t.Fatal(err)
	}
	out, err = runSetupArgs(t, "--codex", "--check")
	if err != nil || !strings.Contains(out, "current") {
		t.Errorf("expected current after setup, got err=%v out:\n%s", err, out)
	}
}

func TestDetectAgents_OptInRequiresFlag(t *testing.T) {
	home := isolatedHome(t)
	mkdirs(t, filepath.Join(home, ".codex"))

	det, err := detectAgents(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(det.Selected) != 0 {
		t.Errorf("expected no selected agents without opt-in, got %v", det.Selected)
	}
	if len(det.OptInAvailable) != 1 || det.OptInAvailable[0].flag != "codex" {
		t.Errorf("expected Codex as available opt-in, got %v", det.OptInAvailable)
	}

	det, err = detectAgents(map[string]bool{"codex": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(det.Selected) != 1 || det.Selected[0].Name != "Codex" || !det.Selected[0].Shared {
		t.Errorf("expected shared Codex agent when opted in, got %v", det.Selected)
	}
	if want := filepath.Join(home, ".codex", "AGENTS.md"); filepath.Join(det.Selected[0].RulesDir, det.Selected[0].RulesFile) != want {
		t.Errorf("unexpected Codex rules path")
	}
}
