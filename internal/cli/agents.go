package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AIAgent represents a supported AI coding agent that verikt can register with.
type AIAgent struct {
	Name      string // "Claude Code", "Cursor", "Windsurf", "Codex", "Antigravity"
	ConfigDir string // expanded path to ~/.claude etc.
	RulesDir  string // expanded path to ~/.claude/rules etc.
	RulesFile string // "verikt.md", or the agent's shared global file ("AGENTS.md")
	// Shared is true when RulesFile is the agent's single global instructions
	// file, which the user also edits. verikt then only writes, checks and
	// removes the section between globalMarkers and never touches the rest.
	Shared bool
}

// agentSpec describes where an agent keeps its global configuration, relative
// to the user's home directory.
type agentSpec struct {
	name      string
	flag      string // opt-in flag name (without "--"); empty for auto-detected agents
	configDir string // presence of this dir means the agent is installed
	rulesDir  string
	rulesFile string
	shared    bool
	// configDirEnv names an environment variable that overrides the
	// home-relative configDir when set. Empty means no override.
	configDirEnv string
}

// optIn reports whether the agent is only configured when explicitly requested.
func (a agentSpec) optIn() bool { return a.flag != "" }

var supportedAgents = []agentSpec{
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
	// Opt-in agents. Their global instructions live in one file the user also
	// edits, so verikt writes a marked section and requires an explicit flag:
	// an existing directory alone never causes verikt to edit that file.
	//
	// These locations have not yet been checked against each vendor's current
	// documentation; https://github.com/diktahq/verikt/issues/19 tracks that.
	{
		name:      "Codex",
		flag:      "codex",
		configDir: ".codex",
		rulesDir:  ".codex",
		rulesFile: "AGENTS.md",
		shared:    true,
	},
	{
		name:      "Antigravity",
		flag:      "antigravity",
		configDir: filepath.Join(".gemini", "antigravity"),
		rulesDir:  ".gemini",
		rulesFile: "GEMINI.md",
		shared:    true,
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

// agentDetection is the result of looking for agents under the home directory.
type agentDetection struct {
	// Selected are the agents setup acts on: every installed auto-detected
	// agent plus every requested opt-in agent that is installed.
	Selected []AIAgent
	// OptInAvailable are installed opt-in agents the user did not request.
	OptInAvailable []agentSpec
	// OptInMissing are requested opt-in agents whose config dir does not exist.
	OptInMissing []agentSpec
}

// detectAgents finds installed agents under the user's home. optIn holds the
// flag names of opt-in agents the user explicitly requested. Agents with no
// existing config dir are never selected — verikt never creates config dirs.
func detectAgents(optIn map[string]bool) (agentDetection, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return agentDetection{}, fmt.Errorf("resolving home directory: %w", err)
	}

	var det agentDetection
	for _, a := range supportedAgents {
		configPath := filepath.Join(home, a.configDir)
		rulesPath := filepath.Join(home, a.rulesDir)
		if a.configDirEnv != "" {
			if override := resolveConfigDirEnv(a.configDirEnv, home, ""); override != "" {
				configPath = override
				rulesPath = filepath.Join(override, "rules")
			}
		}
		_, statErr := os.Stat(configPath)
		installed := statErr == nil
		requested := !a.optIn() || optIn[a.flag]

		switch {
		case installed && requested:
			det.Selected = append(det.Selected, AIAgent{
				Name:      a.name,
				ConfigDir: configPath,
				RulesDir:  rulesPath,
				RulesFile: a.rulesFile,
				Shared:    a.shared,
			})
		case installed && a.optIn():
			det.OptInAvailable = append(det.OptInAvailable, a)
		case !installed && requested && a.optIn():
			det.OptInMissing = append(det.OptInMissing, a)
		}
	}
	return det, nil
}
