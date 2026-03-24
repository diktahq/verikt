package cli

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

//go:embed skills/verikt-init.md
var skillVeriktInit string

type setupFlags struct {
	uninstall bool
	check     bool
	force     bool
}

var sentinelPattern = regexp.MustCompile(`<!-- verikt:global:v([^>]+) -->`)

func newSetupCommand() *cobra.Command {
	flags := &setupFlags{}

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Register verikt with installed AI agents",
		Long:  "Detect installed AI agents and write global architecture guidance files to each one.",
		Example: `  verikt setup             # detect agents, write global files
  verikt setup --check     # report status without writing
  verikt setup --uninstall # remove all verikt global files
  verikt setup --force     # overwrite even if version matches`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd, flags)
		},
	}

	cmd.Flags().BoolVar(&flags.uninstall, "uninstall", false, "Remove verikt global files from all agents")
	cmd.Flags().BoolVar(&flags.check, "check", false, "Report status without writing (exit 1 if any agent is stale)")
	cmd.Flags().BoolVar(&flags.force, "force", false, "Overwrite even if version matches")

	return cmd
}

func runSetup(cmd *cobra.Command, flags *setupFlags) error {
	agents, err := detectInstalledAgents()
	if err != nil {
		return fmt.Errorf("detecting agents: %w", err)
	}

	if len(agents) == 0 {
		cmd.Println("No supported AI agents detected.")
		cmd.Println()
		cmd.Println("To register verikt with your AI agent, install one of these first:")
		cmd.Println("  - Claude Code: https://claude.ai/download")
		cmd.Println("  - Cursor: https://cursor.com")
		cmd.Println("  - Windsurf: https://windsurf.com")
		cmd.Println()
		cmd.Println("Then run: verikt setup")
		return nil
	}

	if flags.uninstall {
		return runUninstall(cmd, agents)
	}

	if flags.check {
		return runSetupCheck(cmd, agents)
	}

	if err := runWrite(cmd, agents, flags.force); err != nil {
		return err
	}

	return installSkills(cmd, agents)
}

func runWrite(cmd *cobra.Command, agents []AIAgent, force bool) error {
	content := GlobalRulesContent(version)
	written := 0

	for _, agent := range agents {
		rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)

		if !force {
			existing, err := os.ReadFile(rulesFile) //nolint:gosec
			if err == nil {
				installedVersion := extractSentinelVersion(string(existing))
				if installedVersion == version {
					cmd.Printf("  = %s \u2014 already current (v%s)\n", agent.Name, version)
					continue
				}
			}
		}

		if err := os.MkdirAll(agent.RulesDir, 0o755); err != nil {
			return fmt.Errorf("creating rules dir for %s: %w", agent.Name, err)
		}

		if err := os.WriteFile(rulesFile, []byte(content), 0o644); err != nil { //nolint:gosec
			return fmt.Errorf("writing rules file for %s: %w", agent.Name, err)
		}

		cmd.Printf("  \u2713 %s \u2014 wrote %s\n", agent.Name, rulesFile)
		written++
	}

	cmd.Println()
	if written > 0 {
		cmd.Printf("Registered with %d agent(s).\n", written)
	} else {
		cmd.Println("All agents already current.")
	}

	return nil
}

func runSetupCheck(cmd *cobra.Command, agents []AIAgent) error {
	stale := 0

	for _, agent := range agents {
		rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)

		existing, err := os.ReadFile(rulesFile) //nolint:gosec
		if err != nil {
			cmd.Printf("  \u2717 %s \u2014 not installed\n", agent.Name)
			stale++
			continue
		}

		installedVersion := extractSentinelVersion(string(existing))
		if installedVersion == version {
			cmd.Printf("  \u2713 %s \u2014 current (v%s)\n", agent.Name, version)
		} else {
			cmd.Printf("  \u2717 %s \u2014 stale (v%s \u2192 v%s)\n", agent.Name, installedVersion, version)
			stale++
		}
	}

	if stale > 0 {
		cmd.Println()
		cmd.Printf("%d agent(s) need update. Run: verikt setup\n", stale)
		return fmt.Errorf("stale")
	}

	return nil
}

func runUninstall(cmd *cobra.Command, agents []AIAgent) error {
	for _, agent := range agents {
		rulesFile := filepath.Join(agent.RulesDir, agent.RulesFile)

		if err := os.Remove(rulesFile); err != nil {
			if os.IsNotExist(err) {
				cmd.Printf("  = %s \u2014 not installed (skipped)\n", agent.Name)
				continue
			}
			return fmt.Errorf("removing %s: %w", rulesFile, err)
		}

		cmd.Printf("  \u2713 %s \u2014 removed %s\n", agent.Name, rulesFile)
	}

	return nil
}

func extractSentinelVersion(content string) string {
	matches := sentinelPattern.FindStringSubmatch(content)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

// installSkills asks the user where to install verikt skills and writes them.
func installSkills(cmd *cobra.Command, agents []AIAgent) error {
	// Only offer skills for Claude Code (skills are a Claude Code feature).
	var claudeAgent *AIAgent
	for i := range agents {
		if agents[i].Name == "Claude Code" {
			claudeAgent = &agents[i]
			break
		}
	}
	if claudeAgent == nil {
		return nil
	}

	cmd.Println()

	var installScope string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Install verikt skills?").
			Description("Skills let you run /verikt:init inside Claude Code.").
			Options(
				huh.NewOption("Global — available in all projects", "global"),
				huh.NewOption("Local — this project only", "local"),
				huh.NewOption("Both — global and local", "both"),
				huh.NewOption("Skip — don't install skills", "skip"),
			).
			Value(&installScope),
	)).Run(); err != nil {
		return nil
	}

	if installScope == "skip" {
		return nil
	}

	skills := map[string]string{
		"verikt:init": skillVeriktInit,
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}

	writeSkill := func(baseDir, name, content string) error {
		dir := filepath.Join(baseDir, "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating skill dir %s: %w", dir, err)
		}
		path := filepath.Join(dir, "SKILL.md")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing skill %s: %w", path, err)
		}
		cmd.Printf("  ✓ %s → %s\n", name, path)
		return nil
	}

	for name, content := range skills {
		if installScope == "global" || installScope == "both" {
			globalBase := filepath.Join(home, ".claude")
			if err := writeSkill(globalBase, name, content); err != nil {
				return err
			}
		}
		if installScope == "local" || installScope == "both" {
			localBase := ".claude"
			if err := writeSkill(localBase, name, content); err != nil {
				return err
			}
		}
	}

	return nil
}
