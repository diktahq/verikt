package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeAgent builds an AIAgent rooted under dir for testing.
func makeAgent(t *testing.T, home, name, configDir, rulesSubDir string) AIAgent {
	t.Helper()
	configPath := filepath.Join(home, configDir)
	rulesPath := filepath.Join(home, rulesSubDir)
	if err := os.MkdirAll(configPath, 0o755); err != nil {
		t.Fatalf("makeAgent: mkdir %s: %v", configPath, err)
	}
	return AIAgent{
		Name:      name,
		ConfigDir: configPath,
		RulesDir:  rulesPath,
		RulesFile: "verikt.md",
	}
}

// findAgent returns the named agent, or fails the test.
func findAgent(t *testing.T, agents []AIAgent, name string) AIAgent {
	t.Helper()
	for _, a := range agents {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("expected %s to be detected, got %v", name, agents)
	return AIAgent{}
}

func TestSetup_DetectsClaudeCode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	// Create ~/.claude so Claude Code is detected.
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	agents, err := detectInstalledAgents()
	if err != nil {
		t.Fatalf("detectInstalledAgents: %v", err)
	}

	agent := findAgent(t, agents, "Claude Code")
	if want := filepath.Join(home, ".claude"); agent.ConfigDir != want {
		t.Errorf("ConfigDir = %q, want %q", agent.ConfigDir, want)
	}
}

func TestSetup_DetectsNoAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Cleared explicitly: a profile alias in the ambient environment would
	// otherwise point at a real config dir and be detected.
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	agents, err := detectInstalledAgents()
	if err != nil {
		t.Fatalf("detectInstalledAgents: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected no agents, got %d: %v", len(agents), agents)
	}
}

// Claude Code profile aliases (claude-supermodular, claude-personal) each point
// CLAUDE_CONFIG_DIR at their own directory. Assuming ~/.claude wrote to — and
// detected — the wrong profile.
func TestSetup_ClaudeConfigDirEnvOverridesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	alias := filepath.Join(home, ".claude-supermodular")
	if err := os.MkdirAll(alias, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", alias)

	agents, err := detectInstalledAgents()
	if err != nil {
		t.Fatalf("detectInstalledAgents: %v", err)
	}

	agent := findAgent(t, agents, "Claude Code")
	if agent.ConfigDir != alias {
		t.Errorf("ConfigDir = %q, want %q", agent.ConfigDir, alias)
	}
	if want := filepath.Join(alias, "rules"); agent.RulesDir != want {
		t.Errorf("RulesDir = %q, want %q", agent.RulesDir, want)
	}
}

// A "~/" prefix in the env var must resolve against home, not be treated as a
// literal directory name.
func TestSetup_ClaudeConfigDirEnvExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	alias := filepath.Join(home, ".claude-personal")
	if err := os.MkdirAll(alias, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "~/.claude-personal")

	agents, err := detectInstalledAgents()
	if err != nil {
		t.Fatalf("detectInstalledAgents: %v", err)
	}

	agent := findAgent(t, agents, "Claude Code")
	if agent.ConfigDir != alias {
		t.Errorf("ConfigDir = %q, want %q", agent.ConfigDir, alias)
	}
}

// A CLAUDE_CONFIG_DIR pointing at a directory that does not exist means the
// profile is not installed — verikt never creates agent config dirs.
func TestSetup_ClaudeConfigDirEnvMissingDirNotDetected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// ~/.claude exists, but the active profile does not — the alias wins.
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude-absent"))

	agents, err := detectInstalledAgents()
	if err != nil {
		t.Fatalf("detectInstalledAgents: %v", err)
	}
	for _, a := range agents {
		if a.Name == "Claude Code" {
			t.Errorf("Claude Code should not be detected, got ConfigDir %q", a.ConfigDir)
		}
	}
}

func TestSetup_WritesRulesFile(t *testing.T) {
	home := t.TempDir()
	agent := makeAgent(t, home, "Claude Code", ".claude", filepath.Join(".claude", "rules"))

	var buf bytes.Buffer
	cmd := newSetupCommand()
	cmd.SetOut(&buf)

	if err := runWrite(cmd, []AIAgent{agent}, false); err != nil {
		t.Fatalf("runWrite: %v", err)
	}

	rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)
	data, err := os.ReadFile(rulesFile) //nolint:gosec
	if err != nil {
		t.Fatalf("expected rules file to exist at %s: %v", rulesFile, err)
	}

	content := string(data)
	sentinel := "<!-- verikt:global:v" + version + " -->"
	if !strings.Contains(content, sentinel) {
		t.Errorf("expected sentinel %q in file", sentinel)
	}
	if !strings.Contains(content, "verikt") {
		t.Error("expected 'verikt' in file content")
	}
	if !strings.Contains(content, "## AI Interview:") {
		t.Error("expected interview section in file content")
	}
}

func TestSetup_Idempotent(t *testing.T) {
	home := t.TempDir()
	agent := makeAgent(t, home, "Claude Code", ".claude", filepath.Join(".claude", "rules"))

	cmd := newSetupCommand()
	cmd.SetOut(&bytes.Buffer{})

	if err := runWrite(cmd, []AIAgent{agent}, false); err != nil {
		t.Fatalf("first runWrite: %v", err)
	}

	rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)
	first, err := os.ReadFile(rulesFile) //nolint:gosec
	if err != nil {
		t.Fatalf("reading first write: %v", err)
	}

	cmd2 := newSetupCommand()
	cmd2.SetOut(&bytes.Buffer{})
	if err := runWrite(cmd2, []AIAgent{agent}, false); err != nil {
		t.Fatalf("second runWrite: %v", err)
	}

	second, err := os.ReadFile(rulesFile) //nolint:gosec
	if err != nil {
		t.Fatalf("reading second write: %v", err)
	}

	if string(first) != string(second) {
		t.Error("expected identical content on second run (idempotent)")
	}
}

func TestSetup_UpgradeDetection(t *testing.T) {
	home := t.TempDir()
	agent := makeAgent(t, home, "Claude Code", ".claude", filepath.Join(".claude", "rules"))

	// Write a file with an old version sentinel.
	rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)
	if err := os.MkdirAll(agent.RulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldContent := "<!-- verikt:global:v0.0.1 -->\nold content\n"
	if err := os.WriteFile(rulesFile, []byte(oldContent), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	cmd := newSetupCommand()
	cmd.SetOut(&bytes.Buffer{})
	if err := runWrite(cmd, []AIAgent{agent}, false); err != nil {
		t.Fatalf("runWrite: %v", err)
	}

	data, err := os.ReadFile(rulesFile) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	expectedSentinel := "<!-- verikt:global:v" + version + " -->"
	if !strings.Contains(content, expectedSentinel) {
		t.Errorf("expected updated sentinel %q after upgrade, got:\n%s", expectedSentinel, content)
	}
	if strings.Contains(content, "old content") {
		t.Error("expected old content to be replaced")
	}
}

func TestSetup_CheckMode_Current(t *testing.T) {
	home := t.TempDir()
	agent := makeAgent(t, home, "Claude Code", ".claude", filepath.Join(".claude", "rules"))

	// Write a file with the current version sentinel.
	rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)
	if err := os.MkdirAll(agent.RulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	currentContent := GlobalRulesContent(version)
	if err := os.WriteFile(rulesFile, []byte(currentContent), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	var buf bytes.Buffer
	cmd := newSetupCommand()
	cmd.SetOut(&buf)

	err := runSetupCheck(cmd, []AIAgent{agent})
	if err != nil {
		t.Errorf("expected no error (current), got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "current") {
		t.Errorf("expected 'current' in output, got: %s", output)
	}
}

func TestSetup_CheckMode_Stale(t *testing.T) {
	home := t.TempDir()
	agent := makeAgent(t, home, "Claude Code", ".claude", filepath.Join(".claude", "rules"))

	// Write a file with an old version sentinel.
	rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)
	if err := os.MkdirAll(agent.RulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldContent := "<!-- verikt:global:v0.0.1 -->\nold content\n"
	if err := os.WriteFile(rulesFile, []byte(oldContent), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	var buf bytes.Buffer
	cmd := newSetupCommand()
	cmd.SetOut(&buf)

	err := runSetupCheck(cmd, []AIAgent{agent})
	if err == nil {
		t.Error("expected error (stale), got nil")
	}

	output := buf.String()
	if !strings.Contains(output, "stale") {
		t.Errorf("expected 'stale' in output, got: %s", output)
	}
}

func TestSetup_Uninstall(t *testing.T) {
	home := t.TempDir()
	agent := makeAgent(t, home, "Claude Code", ".claude", filepath.Join(".claude", "rules"))

	// Write the rules file first.
	rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)
	if err := os.MkdirAll(agent.RulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesFile, []byte("content"), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	// Also create a sibling file to verify the directory is not removed.
	siblingFile := filepath.Join(agent.RulesDir, "other.md")
	if err := os.WriteFile(siblingFile, []byte("keep me"), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	cmd := newSetupCommand()
	cmd.SetOut(&bytes.Buffer{})
	if err := runUninstall(cmd, []AIAgent{agent}); err != nil {
		t.Fatalf("runUninstall: %v", err)
	}

	// verikt.md should be gone.
	if _, err := os.Stat(rulesFile); !os.IsNotExist(err) {
		t.Error("expected rules file to be removed after uninstall")
	}

	// Rules directory should still exist.
	if _, err := os.Stat(agent.RulesDir); os.IsNotExist(err) {
		t.Error("expected rules directory to still exist after uninstall")
	}

	// Sibling file should be untouched.
	if _, err := os.Stat(siblingFile); os.IsNotExist(err) {
		t.Error("expected sibling file to be untouched after uninstall")
	}
}

func TestSetup_ForceOverwrite(t *testing.T) {
	home := t.TempDir()
	agent := makeAgent(t, home, "Claude Code", ".claude", filepath.Join(".claude", "rules"))

	rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)
	if err := os.MkdirAll(agent.RulesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a file with current version but different body to simulate manual edit.
	modifiedContent := "<!-- verikt:global:v" + version + " -->\nmanually modified content\n"
	if err := os.WriteFile(rulesFile, []byte(modifiedContent), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	// Without --force, the file should NOT be overwritten (version matches).
	cmd := newSetupCommand()
	cmd.SetOut(&bytes.Buffer{})
	if err := runWrite(cmd, []AIAgent{agent}, false); err != nil {
		t.Fatalf("runWrite (no force): %v", err)
	}
	data, _ := os.ReadFile(rulesFile) //nolint:gosec
	if !strings.Contains(string(data), "manually modified content") {
		t.Error("expected file to be unchanged without --force when version matches")
	}

	// With --force, the file should be restored to canonical content.
	cmd2 := newSetupCommand()
	cmd2.SetOut(&bytes.Buffer{})
	if err := runWrite(cmd2, []AIAgent{agent}, true); err != nil {
		t.Fatalf("runWrite (force): %v", err)
	}
	data, _ = os.ReadFile(rulesFile) //nolint:gosec
	if strings.Contains(string(data), "manually modified content") {
		t.Error("expected manually modified content to be overwritten by --force")
	}
	canonical := GlobalRulesContent(version)
	if string(data) != canonical {
		t.Error("expected file to match canonical GlobalRulesContent after --force")
	}
}

func TestGlobalRulesContent_TokenBudget(t *testing.T) {
	content := GlobalRulesContent("0.2.0")
	// Approximate tokens: 1 token ≈ 4 chars (conservative).
	approxTokens := len(content) / 4
	if approxTokens > 800 {
		t.Errorf("content exceeds 800 tokens (approx %d tokens, %d chars)", approxTokens, len(content))
	}
}
