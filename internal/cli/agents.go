package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// AIAgent represents a supported AI coding agent that archway can register with.
type AIAgent struct {
	Name      string // "Claude Code", "Cursor", "Windsurf"
	ConfigDir string // expanded path to ~/.claude etc.
	RulesDir  string // expanded path to ~/.claude/rules etc.
	RulesFile string // "archway.md"
}

var supportedAgents = []struct {
	name      string
	configDir string
	rulesDir  string
	rulesFile string
}{
	{
		name:      "Claude Code",
		configDir: ".claude",
		rulesDir:  filepath.Join(".claude", "rules"),
		rulesFile: "archway.md",
	},
	{
		name:      "Cursor",
		configDir: ".cursor",
		rulesDir:  filepath.Join(".cursor", "rules"),
		rulesFile: "archway.md",
	},
	{
		name:      "Windsurf",
		configDir: ".windsurf",
		rulesDir:  filepath.Join(".windsurf", "rules"),
		rulesFile: "archway.md",
	},
}

// detectInstalledAgents returns agents whose ConfigDir exists under the user's home.
// Agents with no existing ConfigDir are skipped — we never create config dirs.
func detectInstalledAgents() ([]AIAgent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving home directory: %w", err)
	}

	var found []AIAgent
	for _, a := range supportedAgents {
		configPath := filepath.Join(home, a.configDir)
		if _, statErr := os.Stat(configPath); statErr != nil {
			continue
		}
		found = append(found, AIAgent{
			Name:      a.name,
			ConfigDir: configPath,
			RulesDir:  filepath.Join(home, a.rulesDir),
			RulesFile: a.rulesFile,
		})
	}

	return found, nil
}
