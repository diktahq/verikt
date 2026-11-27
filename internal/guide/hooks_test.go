package guide

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// --- Hook script execution -------------------------------------------------
//
// The hook scripts are bash programs embedded as Go strings, so a substring
// assertion on their text proved nothing: the PostToolUse hook shipped with
// `|| true` masking the check exit code and never reported a single violation.
// These tests run the scripts with a fake `verikt` on PATH.

// requireTools skips only when a tool the script itself needs is missing.
func requireTools(t *testing.T, tools ...string) map[string]string {
	t.Helper()
	paths := map[string]string{}
	for _, tool := range tools {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s not available: %v", tool, err)
		}
		paths[tool] = p
	}
	return paths
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
}

// runHook runs script with bash in dir, feeding stdin, with PATH set to path.
func runHook(t *testing.T, bash, script, dir, path, stdin string) (string, string, error) {
	t.Helper()
	scriptPath := filepath.Join(t.TempDir(), "hook.sh")
	writeExecutable(t, scriptPath, script)
	cmd := exec.CommandContext(t.Context(), bash, scriptPath)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + path, "HOME=" + t.TempDir()}
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// fakeVeriktCheck installs a verikt that prints report and exits with code.
func fakeVeriktCheck(t *testing.T, report string, code int) string {
	t.Helper()
	bin := t.TempDir()
	reportFile := filepath.Join(bin, "report.json")
	require.NoError(t, os.WriteFile(reportFile, []byte(report), 0o644))
	writeExecutable(t, filepath.Join(bin, "verikt"), fmt.Sprintf(
		"#!/bin/sh\ncat '%s'\nexit %d\n", reportFile, code))
	return bin
}

type hookOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func decodeHookOutput(t *testing.T, stdout string) hookOutput {
	t.Helper()
	var out hookOutput
	require.NoError(t, json.Unmarshal([]byte(stdout), &out), "hook output must be valid JSON, got %q", stdout)
	return out
}

func postToolUseInput(t *testing.T, filePath string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"tool_input": map[string]any{"file_path": filePath}})
	require.NoError(t, err)
	return string(data)
}

// Every array `verikt check -o json` reports a finding in counts toward the total.
func TestHookScriptCheck_CountsEveryViolationCategory(t *testing.T) {
	tools := requireTools(t, "bash", "jq")
	report := `{
  "schema_version": 2, "result": "fail",
  "violations": [{"rule": "a"}, {"rule": "b"}],
  "anti_patterns": [{"name": "c"}],
  "waived": [{"rule": "not counted"}],
  "proxy_rules": {"violations": [{"rule": "d"}], "statuses": []},
  "decision_violations": [{"decision": "e"}]
}`
	bin := fakeVeriktCheck(t, report, 1)

	stdout, stderr, err := runHook(t, tools["bash"], hookScriptCheck, t.TempDir(),
		bin+":"+os.Getenv("PATH"), postToolUseInput(t, "internal/app/service.go"))
	require.NoError(t, err, "stderr: %s", stderr)

	out := decodeHookOutput(t, stdout)
	assert.Equal(t, "PostToolUse", out.HookSpecificOutput.HookEventName)
	assert.Contains(t, out.HookSpecificOutput.AdditionalContext, "found 5 violation(s)")
	assert.Contains(t, out.HookSpecificOutput.AdditionalContext, "internal/app/service.go")
}

// A file path containing quotes and backslashes must not break the JSON.
func TestHookScriptCheck_EscapesFilePathInOutput(t *testing.T) {
	tools := requireTools(t, "bash", "jq")
	bin := fakeVeriktCheck(t, `{"violations": [{"rule": "a"}], "anti_patterns": []}`, 1)
	nasty := `dir/we"ird\name.go`

	stdout, stderr, err := runHook(t, tools["bash"], hookScriptCheck, t.TempDir(),
		bin+":"+os.Getenv("PATH"), postToolUseInput(t, nasty))
	require.NoError(t, err, "stderr: %s", stderr)

	out := decodeHookOutput(t, stdout)
	assert.Contains(t, out.HookSpecificOutput.AdditionalContext, nasty)
}

func TestHookScriptCheck_SilentWhenCheckPasses(t *testing.T) {
	tools := requireTools(t, "bash", "jq")
	bin := fakeVeriktCheck(t, `{"result": "pass", "violations": [], "anti_patterns": []}`, 0)

	stdout, stderr, err := runHook(t, tools["bash"], hookScriptCheck, t.TempDir(),
		bin+":"+os.Getenv("PATH"), postToolUseInput(t, "main.go"))
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Empty(t, stdout)
}

// A check that fails without a readable report is still surfaced: silence would
// claim compliance that was never verified.
func TestHookScriptCheck_ReportsFailureWithoutReadableReport(t *testing.T) {
	tools := requireTools(t, "bash", "jq")
	bin := fakeVeriktCheck(t, "engine crashed", 3)

	stdout, stderr, err := runHook(t, tools["bash"], hookScriptCheck, t.TempDir(),
		bin+":"+os.Getenv("PATH"), postToolUseInput(t, "main.go"))
	require.NoError(t, err, "stderr: %s", stderr)

	out := decodeHookOutput(t, stdout)
	assert.Equal(t, "PostToolUse", out.HookSpecificOutput.HookEventName)
	assert.Contains(t, out.HookSpecificOutput.AdditionalContext, "exit code 3")
}

// LLM-F2: a check that fails only because a proxy rule could not run (stale or
// invalid) has no findings, and must say so instead of claiming the report was
// unreadable.
func TestHookScriptCheck_ReportsUnrunProxyRules(t *testing.T) {
	tools := requireTools(t, "bash", "jq")
	report := `{
  "result": "fail", "violations": [], "anti_patterns": [],
  "proxy_rules": {"violations": [], "statuses": [
    {"filename": "ok.yaml", "status": "valid"},
    {"filename": "no-raw-sql.yaml", "status": "stale"},
    {"filename": "bad\"name.yaml", "status": "invalid", "error": "missing pattern"}
  ]}
}`
	bin := fakeVeriktCheck(t, report, 1)

	stdout, stderr, err := runHook(t, tools["bash"], hookScriptCheck, t.TempDir(),
		bin+":"+os.Getenv("PATH"), postToolUseInput(t, "main.go"))
	require.NoError(t, err, "stderr: %s", stderr)

	ctx := decodeHookOutput(t, stdout).HookSpecificOutput.AdditionalContext
	assert.Contains(t, ctx, "2 proxy rule(s) could not run")
	assert.Contains(t, ctx, "no-raw-sql.yaml")
	assert.Contains(t, ctx, `bad"name.yaml`)
	assert.NotContains(t, ctx, "no readable report")
	assert.NotContains(t, ctx, "ok.yaml")
}

// Findings and unrun rules in one run are both reported.
func TestHookScriptCheck_ReportsViolationsAndUnrunRules(t *testing.T) {
	tools := requireTools(t, "bash", "jq")
	report := `{"violations": [{"rule": "a"}], "anti_patterns": [],
  "proxy_rules": {"violations": [], "statuses": [{"filename": "x.yaml", "status": "stale"}]}}`
	bin := fakeVeriktCheck(t, report, 1)

	stdout, stderr, err := runHook(t, tools["bash"], hookScriptCheck, t.TempDir(),
		bin+":"+os.Getenv("PATH"), postToolUseInput(t, "main.go"))
	require.NoError(t, err, "stderr: %s", stderr)

	ctx := decodeHookOutput(t, stdout).HookSpecificOutput.AdditionalContext
	assert.Contains(t, ctx, "found 1 violation(s)")
	assert.Contains(t, ctx, "1 proxy rule(s) could not run")
}

func TestHookScriptCheck_SkipsVendorFiles(t *testing.T) {
	tools := requireTools(t, "bash", "jq")
	bin := fakeVeriktCheck(t, `{"violations": [{"rule": "a"}]}`, 1)

	stdout, stderr, err := runHook(t, tools["bash"], hookScriptCheck, t.TempDir(),
		bin+":"+os.Getenv("PATH"), postToolUseInput(t, "/repo/vendor/x/y.go"))
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Empty(t, stdout)
}

// refreshPath builds a PATH holding only what the refresh script needs, plus
// the fakes in extra, so the presence of sha256sum and shasum is controlled.
func refreshPath(t *testing.T, extra map[string]string) string {
	t.Helper()
	tools := requireTools(t, "cat", "cut", "mktemp", "mv", "mkdir")
	bin := t.TempDir()
	for name, p := range tools {
		require.NoError(t, os.Symlink(p, filepath.Join(bin, name)))
	}
	for name, content := range extra {
		writeExecutable(t, filepath.Join(bin, name), content)
	}
	return bin
}

const fakeVeriktGuide = "#!/bin/sh\n[ \"$1\" = guide ] && exit 0\nexit 9\n"

func refreshProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "verikt.yaml"), []byte("architecture: flat\n"), 0o644))
	return dir
}

func TestHookScriptGuideRefresh_UsesSha256sum(t *testing.T) {
	tools := requireTools(t, "bash")
	dir := refreshProject(t)
	path := refreshPath(t, map[string]string{
		"verikt":    fakeVeriktGuide,
		"sha256sum": "#!/bin/sh\necho \"fromsha256sum  $1\"\n",
	})

	stdout, stderr, err := runHook(t, tools["bash"], hookScriptGuideRefresh, dir, path, "")
	require.NoError(t, err, "stderr: %s", stderr)

	hash, err := os.ReadFile(filepath.Join(dir, ".verikt", ".guide-hash"))
	require.NoError(t, err)
	assert.Equal(t, "fromsha256sum", strings.TrimSpace(string(hash)))
	assert.Equal(t, "SessionStart", decodeHookOutput(t, stdout).HookSpecificOutput.HookEventName)
}

func TestHookScriptGuideRefresh_FallsBackToShasum(t *testing.T) {
	tools := requireTools(t, "bash")
	dir := refreshProject(t)
	path := refreshPath(t, map[string]string{
		"verikt": fakeVeriktGuide,
		"shasum": "#!/bin/sh\n[ \"$1\" = -a ] && [ \"$2\" = 256 ] || exit 2\necho \"fromshasum  $3\"\n",
	})

	_, stderr, err := runHook(t, tools["bash"], hookScriptGuideRefresh, dir, path, "")
	require.NoError(t, err, "stderr: %s", stderr)

	hash, err := os.ReadFile(filepath.Join(dir, ".verikt", ".guide-hash"))
	require.NoError(t, err)
	assert.Equal(t, "fromshasum", strings.TrimSpace(string(hash)))
}

func TestHookScriptGuideRefresh_SkipsWhenHashIsFresh(t *testing.T) {
	tools := requireTools(t, "bash")
	dir := refreshProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".verikt"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".verikt", ".guide-hash"), []byte("same"), 0o644))
	path := refreshPath(t, map[string]string{
		"verikt":    "#!/bin/sh\necho called >&2\nexit 9\n",
		"sha256sum": "#!/bin/sh\necho \"same  $1\"\n",
	})

	stdout, stderr, err := runHook(t, tools["bash"], hookScriptGuideRefresh, dir, path, "")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Empty(t, stdout)
	assert.NotContains(t, stderr, "called", "a fresh guide must not be regenerated")
}

func TestHookScriptGuideRefresh_NoHasherDoesNotFail(t *testing.T) {
	tools := requireTools(t, "bash")
	dir := refreshProject(t)
	path := refreshPath(t, map[string]string{"verikt": fakeVeriktGuide})

	stdout, stderr, err := runHook(t, tools["bash"], hookScriptGuideRefresh, dir, path, "")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "sha256sum")
}

// --- settings.json merge ---------------------------------------------------

func writeSettings(t *testing.T, dir, content string) string {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	p := filepath.Join(claudeDir, "settings.json")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func veriktCheckHooks(timeout int) map[string]any {
	return map[string]any{
		"PostToolUse": []any{
			map[string]any{
				"matcher": "Write|Edit",
				"hooks": []any{map[string]any{
					"type":    "command",
					"command": `"$CLAUDE_PROJECT_DIR"/.claude/hooks/verikt-check.sh`,
					"timeout": timeout,
				}},
			},
		},
	}
}

// A settings.json verikt cannot parse belongs to the user; overwriting it with
// verikt's hooks destroyed everything else it held.
func TestMergeClaudeSettingsJSON_RefusesInvalidJSON(t *testing.T) {
	for name, content := range map[string]string{
		"syntax error":     `{"permissions": {"allow": ["Bash"]},`,
		"top-level array":  `["not", "an", "object"]`,
		"top-level null":   `null`,
		"hooks not object": `{"hooks": ["x"]}`,
		"event not array":  `{"hooks": {"PostToolUse": {"matcher": "x"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p := writeSettings(t, dir, content)

			err := mergeClaudeSettingsJSON(dir, veriktCheckHooks(10))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "settings.json")

			after, readErr := os.ReadFile(p)
			require.NoError(t, readErr)
			assert.Equal(t, content, string(after), "the user's file must be left untouched")
		})
	}
}

func TestGenerateFromConfig_FailsOnInvalidSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{not json`)
	err := GenerateFromConfig(dir, &config.VeriktConfig{Architecture: "flat"}, "claude")
	require.Error(t, err)
}

// A changed verikt entry (new timeout, say) replaces the old one in place
// rather than being appended next to it.
func TestMergeClaudeSettingsJSON_UpdatesVeriktEntryInPlace(t *testing.T) {
	dir := t.TempDir()
	p := writeSettings(t, dir, `{"theme": "dark", "hooks": {"PostToolUse": [
	  {"matcher": "Bash", "hooks": [{"type": "command", "command": "user-before.sh"}]},
	  {"matcher": "Write", "hooks": [{"type": "command", "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/verikt-check.sh", "timeout": 5}]},
	  {"matcher": "Edit", "hooks": [{"type": "command", "command": "user-after.sh"}]}
	]}}`)

	require.NoError(t, mergeClaudeSettingsJSON(dir, veriktCheckHooks(10)))
	require.NoError(t, mergeClaudeSettingsJSON(dir, veriktCheckHooks(10)))

	assert.Equal(t, []string{
		"user-before.sh",
		`"$CLAUDE_PROJECT_DIR"/.claude/hooks/verikt-check.sh`,
		"user-after.sh",
	}, hookCommandsForEvent(t, p, "PostToolUse"))

	var got map[string]any
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "dark", got["theme"])
	entry := got["hooks"].(map[string]any)["PostToolUse"].([]any)[1].(map[string]any)
	assert.Equal(t, "Write|Edit", entry["matcher"])
	assert.InDelta(t, 10, entry["hooks"].([]any)[0].(map[string]any)["timeout"], 0)
}

// A user hook sharing a matcher group with verikt's survives the update.
func TestMergeClaudeSettingsJSON_KeepsUserHookInSharedGroup(t *testing.T) {
	dir := t.TempDir()
	p := writeSettings(t, dir, `{"hooks": {"PostToolUse": [
	  {"matcher": "Write|Edit", "hooks": [
	    {"type": "command", "command": "fmt.sh"},
	    {"type": "command", "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/verikt-check.sh", "timeout": 5}
	  ]}
	]}}`)

	require.NoError(t, mergeClaudeSettingsJSON(dir, veriktCheckHooks(10)))

	commands := hookCommandsForEvent(t, p, "PostToolUse")
	assert.ElementsMatch(t, []string{"fmt.sh", `"$CLAUDE_PROJECT_DIR"/.claude/hooks/verikt-check.sh`}, commands)
}

// The write goes through a temp file and rename: no temp file is left behind
// and the file keeps the permissions the user gave it.
func TestMergeClaudeSettingsJSON_WritesAtomicallyKeepingMode(t *testing.T) {
	dir := t.TempDir()
	p := writeSettings(t, dir, `{"theme": "dark"}`)

	require.NoError(t, mergeClaudeSettingsJSON(dir, veriktCheckHooks(10)))

	info, err := os.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	entries, err := os.ReadDir(filepath.Dir(p))
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"settings.json"}, names)
}

func TestMergeClaudeSettingsJSON_CreatesClaudeDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, mergeClaudeSettingsJSON(dir, veriktCheckHooks(10)))
	assert.FileExists(t, filepath.Join(dir, ".claude", "settings.json"))
}

// LLM-F2: a failed regeneration is reported to the agent through SessionStart
// additionalContext, and the stored hash is left alone so the next session
// retries.
func TestHookScriptGuideRefresh_ReportsGuideFailure(t *testing.T) {
	tools := requireTools(t, "bash")
	dir := refreshProject(t)
	path := refreshPath(t, map[string]string{
		"verikt":    "#!/bin/sh\necho 'load verikt.yaml: boom' >&2\nexit 1\n",
		"sha256sum": "#!/bin/sh\necho \"newhash  $1\"\n",
	})

	stdout, stderr, err := runHook(t, tools["bash"], hookScriptGuideRefresh, dir, path, "")
	require.NoError(t, err, "stderr: %s", stderr)

	out := decodeHookOutput(t, stdout)
	assert.Equal(t, "SessionStart", out.HookSpecificOutput.HookEventName)
	assert.Contains(t, out.HookSpecificOutput.AdditionalContext, "verikt guide --target claude failed")
	assert.NoFileExists(t, filepath.Join(dir, ".verikt", ".guide-hash"))
}
