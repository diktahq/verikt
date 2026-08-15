package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AIAgent represents a supported AI coding agent that verikt can register with.
type AIAgent struct {
	Name      string // "Claude Code", "Cursor", "Windsurf"
	ConfigDir string // expanded path to ~/.claude etc.
	RulesDir  string // expanded path to ~/.claude/rules etc.
	RulesFile string // "verikt.md"
}

var supportedAgents = []struct {
	name      string
	configDir string
	rulesDir  string
	rulesFile string
	// configDirEnv names an environment variable that overrides the
	// home-relative configDir when set. Empty means no override.
	configDirEnv string
}{
	{
		name:         "Claude Code",
		configDir:    ".claude",
		rulesDir:     filepath.Join(".claude", "rules"),
		rulesFile:    "verikt.md",
		configDirEnv: claudeConfigDirEnv,
	},
	{
		name:      "Cursor",
		configDir: ".cursor",
		rulesDir:  filepath.Join(".cursor", "rules"),
		rulesFile: "verikt.md",
	},
	{
		name:      "Windsurf",
		configDir: ".windsurf",
		rulesDir:  filepath.Join(".windsurf", "rules"),
		rulesFile: "verikt.md",
	},
}

// claudeConfigDirEnv is the environment variable Claude Code uses to select a
// profile. Aliases such as claude-supermodular or claude-personal each set it to
// their own directory, so ~/.claude is only the default, not the only location.
const claudeConfigDirEnv = "CLAUDE_CONFIG_DIR"

// claudeGlobalConfigDir returns the active Claude Code global config directory:
// CLAUDE_CONFIG_DIR when set, otherwise ~/.claude.
func claudeGlobalConfigDir(home string) string {
	return resolveConfigDirEnv(claudeConfigDirEnv, home, filepath.Join(home, ".claude"))
}

// resolveConfigDirEnv reads a config-dir environment variable, expanding a "~/"
// prefix against home. It returns fallback when the variable is unset or blank.
func resolveConfigDirEnv(envVar, home, fallback string) string {
	dir := strings.TrimSpace(os.Getenv(envVar))
	if dir == "" {
		return fallback
	}
	if dir == "~" {
		return home
	}
	if strings.HasPrefix(dir, "~/") {
		return filepath.Join(home, strings.TrimPrefix(dir, "~/"))
	}
	return dir
}

// detectInstalledAgents returns agents whose ConfigDir exists. Agents with no
// existing ConfigDir are skipped — we never create config dirs.
func detectInstalledAgents() ([]AIAgent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving home directory: %w", err)
	}

	found := make([]AIAgent, 0, len(supportedAgents))
	for _, a := range supportedAgents {
		configPath := filepath.Join(home, a.configDir)
		rulesPath := filepath.Join(home, a.rulesDir)
		if a.configDirEnv != "" {
			if override := resolveConfigDirEnv(a.configDirEnv, home, ""); override != "" {
				configPath = override
				rulesPath = filepath.Join(override, "rules")
			}
		}
		if _, statErr := os.Stat(configPath); statErr != nil {
			continue
		}
		found = append(found, AIAgent{
			Name:      a.name,
			ConfigDir: configPath,
			RulesDir:  rulesPath,
			RulesFile: a.rulesFile,
		})
	}

	return found, nil
}
