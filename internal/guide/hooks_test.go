package guide

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/verikt/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeTarget_GeneratesHookScripts(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.VeriktConfig{Architecture: "flat"}

	err := GenerateFromConfig(dir, cfg, "claude")
	require.NoError(t, err)

	checkScript := filepath.Join(dir, ".claude", "hooks", "verikt-check.sh")
	refreshScript := filepath.Join(dir, ".claude", "hooks", "verikt-guide-refresh.sh")

	assert.FileExists(t, checkScript)
	assert.FileExists(t, refreshScript)

	// Both scripts must be executable (owner execute bit).
	checkInfo, err := os.Stat(checkScript)
	require.NoError(t, err)
	assert.NotZero(t, checkInfo.Mode()&0o100, "verikt-check.sh should be executable")

	refreshInfo, err := os.Stat(refreshScript)
	require.NoError(t, err)
	assert.NotZero(t, refreshInfo.Mode()&0o100, "verikt-guide-refresh.sh should be executable")
}

func TestClaudeTarget_HookScriptsHaveCorrectContent(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.VeriktConfig{Architecture: "flat"}

	require.NoError(t, GenerateFromConfig(dir, cfg, "claude"))

	checkData, err := os.ReadFile(filepath.Join(dir, ".claude", "hooks", "verikt-check.sh"))
	require.NoError(t, err)
	checkContent := string(checkData)
	assert.Contains(t, checkContent, "verikt check --diff HEAD -o json")
	assert.Contains(t, checkContent, "additionalContext")

	refreshData, err := os.ReadFile(filepath.Join(dir, ".claude", "hooks", "verikt-guide-refresh.sh"))
	require.NoError(t, err)
	refreshContent := string(refreshData)
	assert.Contains(t, refreshContent, ".verikt/.guide-hash")
	assert.Contains(t, refreshContent, "verikt guide --target claude")
}

func TestClaudeTarget_WritesSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.VeriktConfig{Architecture: "flat"}

	require.NoError(t, GenerateFromConfig(dir, cfg, "claude"))

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	assert.FileExists(t, settingsPath)

	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	var settings map[string]any
	require.NoError(t, json.Unmarshal(data, &settings))

	hooks, ok := settings["hooks"].(map[string]any)
	require.True(t, ok, "hooks key should be a map")
	assert.Contains(t, hooks, "PostToolUse")
	assert.Contains(t, hooks, "SessionStart")
}

func TestClaudeTarget_MergesExistingSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))

	// Write existing settings with a user permission.
	existing := map[string]any{
		"permissions": map[string]any{
			"allow": []string{"bash"},
		},
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), append(data, '\n'), 0o644))

	cfg := &config.VeriktConfig{Architecture: "flat"}
	require.NoError(t, GenerateFromConfig(dir, cfg, "claude"))

	settingsData, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(settingsData, &result))

	// Existing permissions must be preserved.
	perms, ok := result["permissions"].(map[string]any)
	require.True(t, ok, "permissions should be preserved")
	assert.Contains(t, perms, "allow")

	// Hooks must be added.
	hooks, ok := result["hooks"].(map[string]any)
	require.True(t, ok, "hooks should be added")
	assert.Contains(t, hooks, "PostToolUse")
	assert.Contains(t, hooks, "SessionStart")
}

func TestClaudeTarget_WritesGuideHash(t *testing.T) {
	dir := t.TempDir()

	// Write an verikt.yaml so the hash can be computed.
	yamlContent := "architecture: flat\ncapabilities: []\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "verikt.yaml"), []byte(yamlContent), 0o644))

	cfg := &config.VeriktConfig{Architecture: "flat"}
	require.NoError(t, GenerateFromConfig(dir, cfg, "claude"))

	hashFile := filepath.Join(dir, ".verikt", ".guide-hash")
	assert.FileExists(t, hashFile)

	hashData, err := os.ReadFile(hashFile)
	require.NoError(t, err)

	// Verify hash matches sha256 of verikt.yaml.
	sum := sha256.Sum256([]byte(yamlContent))
	expectedHash := fmt.Sprintf("%x", sum)
	assert.Equal(t, expectedHash, string(hashData))
}

func TestClaudeTarget_NoVeriktYAML_NoHashFile(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.VeriktConfig{Architecture: "flat"}

	// No verikt.yaml — hash file should not be written.
	require.NoError(t, GenerateFromConfig(dir, cfg, "claude"))

	hashFile := filepath.Join(dir, ".verikt", ".guide-hash")
	assert.NoFileExists(t, hashFile)
}

func TestMergeClaudeSettingsJSON_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))

	hooks := map[string]any{
		"PostToolUse": []any{"hook"},
	}
	require.NoError(t, mergeClaudeSettingsJSON(dir, hooks))

	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result))
	hooksMap, ok := result["hooks"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, hooksMap, "PostToolUse")
}

func TestMergeClaudeSettingsJSON_NoExistingFile(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))

	hooks := map[string]any{
		"SessionStart": []any{"hook"},
	}
	require.NoError(t, mergeClaudeSettingsJSON(dir, hooks))

	assert.FileExists(t, filepath.Join(claudeDir, "settings.json"))
}

func TestMergeClaudeSettingsJSON_PreservesExistingHooks(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))

	// Pre-existing settings with a user-defined hook.
	initial := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{"some-user-hook"},
		},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), append(data, '\n'), 0o644))

	archwaHooks := map[string]any{
		"PostToolUse": []any{"verikt-hook"},
	}
	require.NoError(t, mergeClaudeSettingsJSON(dir, archwaHooks))

	merged, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(merged, &result))

	hooksMap := result["hooks"].(map[string]any)
	assert.Contains(t, hooksMap, "PreToolUse", "existing user hook should be preserved")
	assert.Contains(t, hooksMap, "PostToolUse", "verikt hook should be added")
}
