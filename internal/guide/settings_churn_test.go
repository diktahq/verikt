package guide

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settings.json holds the user's own hooks. Re-marshalling reorders keys and
// reformats fields, so writing unconditionally produced a large diff on a file the
// run had not changed, burying real edits in churn.
func TestMergeClaudeSettingsJSON_NoWriteWhenUnchanged(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Hand-written formatting: keys in a different order to Go's marshaller.
	original := "{\n  \"hooks\": {\n    \"SessionStart\": [\n      {\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"mine.sh\"\n          }\n        ]\n      }\n    ]\n  }\n}\n"
	require.NoError(t, os.WriteFile(settingsPath, []byte(original), 0o644))

	existing := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(original), &existing))
	hooks, _ := existing["hooks"].(map[string]any)

	// Merging the hooks already present must not rewrite the file.
	require.NoError(t, mergeClaudeSettingsJSON(dir, hooks))

	after, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Equal(t, original, string(after), "file was rewritten despite no semantic change")
}

// A genuinely new hook is still merged in, and existing settings survive.
func TestMergeClaudeSettingsJSON_WritesNewHooks(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	settingsPath := filepath.Join(claudeDir, "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{"hooks":{"SessionStart":["mine"]},"other":"kept"}`), 0o644))

	require.NoError(t, mergeClaudeSettingsJSON(dir, map[string]any{"PostToolUse": []any{"verikt"}}))

	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, "kept", got["other"], "unrelated settings must survive")
	hooks := got["hooks"].(map[string]any)
	assert.Contains(t, hooks, "SessionStart", "existing hooks must survive")
	assert.Contains(t, hooks, "PostToolUse")
}

// Assigning an event outright destroyed hooks the user already had: a project with
// its own PostToolUse hook silently lost it on every guide run.
func TestMergeClaudeSettingsJSON_PreservesExistingHooksForSameEvent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{
      "hooks": {
        "PostToolUse": [
          {"hooks": [{"type": "command", "command": "$HOME/.other-tool/post-tool-use.sh"}]}
        ]
      }
    }`), 0o644))

	veriktHook := map[string]any{
		"PostToolUse": []any{
			map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "verikt-check.sh"}}},
		},
	}
	require.NoError(t, mergeClaudeSettingsJSON(dir, veriktHook))

	commands := hookCommandsForEvent(t, settingsPath, "PostToolUse")
	assert.Contains(t, commands, "$HOME/.other-tool/post-tool-use.sh", "the existing hook must survive")
	assert.Contains(t, commands, "verikt-check.sh")

	// Running again must not duplicate the entry.
	require.NoError(t, mergeClaudeSettingsJSON(dir, veriktHook))
	assert.Equal(t, commands, hookCommandsForEvent(t, settingsPath, "PostToolUse"), "merge must be idempotent")
}

func hookCommandsForEvent(t *testing.T, settingsPath, event string) []string {
	t.Helper()
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	var parsed struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(data, &parsed))
	var commands []string
	for _, entry := range parsed.Hooks[event] {
		for _, h := range entry.Hooks {
			commands = append(commands, h.Command)
		}
	}
	return commands
}
